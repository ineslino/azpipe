package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ineslino/azpipe/internal/azdo"
	"github.com/ineslino/azpipe/internal/runner"
	"github.com/spf13/cobra"
)

type batchSelection struct {
	ID         int               `json:"id"`
	Mode       runner.Mode       `json:"mode"`
	Branch     string            `json:"branch"`
	Parameters map[string]string `json:"parameters"`
}

type batchRecord struct {
	PipelineID int              `json:"pipelineId"`
	Run        azdo.PipelineRun `json:"run"`
	Error      string           `json:"error,omitempty"`
}

type batchJournal struct {
	Organization string        `json:"organization"`
	Project      string        `json:"project"`
	Runs         []batchRecord `json:"runs"`
}

func init() {
	var file, journalPath, profileName string
	var execute bool
	command := &cobra.Command{Use: "batch", Short: "Rever um lote JSON; --execute confirma a execução", Args: cobra.NoArgs}
	command.Flags().StringVar(&file, "file", "", "Ficheiro JSON de seleções")
	command.Flags().StringVar(&profileName, "profile", "", "Nome de um perfil guardado na TUI")
	command.Flags().StringVar(&journalPath, "journal", "", "Registo novo de runs (obrigatório para executar)")
	command.Flags().BoolVar(&execute, "execute", false, "Confirmar explicitamente o lançamento após validar todo o lote")
	command.RunE = func(cmd *cobra.Command, _ []string) error {
		if (file == "") == (profileName == "") {
			return fmt.Errorf("escolha exactamente um: --file ou --profile")
		}
		var input []batchSelection
		if file != "" {
			f, err := os.Open(file)
			if err != nil {
				return err
			}
			defer f.Close()
			d := json.NewDecoder(f)
			d.DisallowUnknownFields()
			if err := d.Decode(&input); err != nil {
				return err
			}
		}
		client, err := newClient()
		if err != nil {
			return err
		}
		project, err := resolveProject()
		if err != nil {
			return err
		}
		org, err := resolveOrg()
		if err != nil {
			return err
		}
		if profileName != "" {
			profiles, err := runner.ListProfiles(org, project)
			if err != nil {
				return err
			}
			for _, profile := range profiles {
				if profile.Name == profileName {
					for _, s := range profile.Selections {
						input = append(input, batchSelection{ID: s.ID, Mode: s.Mode, Branch: s.Branch, Parameters: s.Parameters})
					}
				}
			}
			if len(input) == 0 {
				return fmt.Errorf("perfil não encontrado neste contexto")
			}
		}
		if len(input) == 0 || len(input) > 500 {
			return fmt.Errorf("lote deve ter 1 a 500 pipelines")
		}
		pipelines, err := client.ListPipelines(cmd.Context(), project)
		if err != nil {
			return err
		}
		byID := map[int]azdo.Pipeline{}
		for _, p := range pipelines {
			byID[p.ID] = p
		}
		seen := map[int]bool{}
		var selections []runner.Selection
		for _, row := range input {
			p, ok := byID[row.ID]
			if !ok || seen[row.ID] {
				return fmt.Errorf("pipeline ausente ou repetida: %d", row.ID)
			}
			seen[row.ID] = true
			if row.Mode == "" {
				row.Mode = runner.ModeRun
			}
			if row.Mode != runner.ModeRun && row.Mode != runner.ModePlan {
				return fmt.Errorf("modo inválido: %s", row.Mode)
			}
			selections = append(selections, runner.Selection{Pipeline: p, Mode: row.Mode, Branch: row.Branch, Inputs: row.Parameters})
		}
		service := runner.NewService(client, project)
		reviews := service.PreviewAll(cmd.Context(), selections, 4)
		fmt.Fprintf(cmd.OutOrStdout(), "Organização: %s | Projecto: %s\n", org, project)
		for _, r := range reviews {
			fmt.Fprintf(cmd.OutOrStdout(), "%s %d %s SHA=%s definição=%d enviados=%v defaults=pipeline\n", r.State, r.Selection.ID(), r.Selection.Mode, r.Request.Commit, r.Request.DefinitionVersion, r.Request.Parameters)
			if r.Err != nil {
				return r.Err
			}
		}
		if !execute {
			return nil
		}
		if journalPath == "" {
			return fmt.Errorf("--execute requer --journal para registar IDs e evitar repetição cega")
		}
		out, err := os.OpenFile(journalPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		defer out.Close()
		if err := out.Close(); err != nil {
			return err
		}
		journal := batchJournal{Organization: org, Project: project, Runs: make([]batchRecord, len(reviews))}
		for i, r := range reviews {
			journal.Runs[i] = batchRecord{PipelineID: r.Selection.ID(), Error: "estado desconhecido: verificar Azure DevOps antes de repetir"}
		}
		save := func() error {
			f, err := os.CreateTemp(filepath.Dir(journalPath), ".azpipe-*.tmp")
			if err != nil {
				return err
			}
			defer os.Remove(f.Name())
			if err := json.NewEncoder(f).Encode(journal); err != nil {
				f.Close()
				return err
			}
			if err := f.Sync(); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
			return os.Rename(f.Name(), journalPath)
		}
		if err := save(); err != nil {
			return err
		}
		var mu sync.Mutex
		service.OnResult = func(i int, r runner.RunResult) error {
			mu.Lock()
			defer mu.Unlock()
			journal.Runs[i].Run = r.Run
			if r.Err == nil && r.Run.ID != 0 {
				journal.Runs[i].Error = ""
			} else {
				journal.Runs[i].Error = fmt.Sprintf("resultado da submissão incerto: %v; verificar antes de repetir", r.Err)
			}
			return save()
		}
		_, err = service.QueueAll(cmd.Context(), reviews, 4)
		if err != nil {
			return err
		}
		return monitorJournal(cmd.Context(), cmd, client, &journal, save)
	}
	rootCmd.AddCommand(command)
	rootCmd.AddCommand(&cobra.Command{Use: "resume <journal>", Short: "Retomar acompanhamento, nunca voltar a lançar runs", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		f, err := os.Open(args[0])
		if err != nil {
			return err
		}
		defer f.Close()
		var journal batchJournal
		if err := json.NewDecoder(f).Decode(&journal); err != nil {
			return err
		}
		org, err := resolveOrg()
		if err != nil {
			return err
		}
		project, err := resolveProject()
		if err != nil {
			return err
		}
		journalOrg, err := validatedOrgURL(journal.Organization)
		if err != nil {
			return err
		}
		if org != journalOrg || project != journal.Project {
			return fmt.Errorf("contexto não corresponde ao registo")
		}
		client, err := newClient()
		if err != nil {
			return err
		}
		return monitorJournal(cmd.Context(), cmd, client, &journal, func() error { return nil })
	}})
}

func monitorJournal(ctx context.Context, cmd *cobra.Command, client azdo.Client, journal *batchJournal, save func() error) error {
	for {
		pending := false
		var failures []error
		for i := range journal.Runs {
			r := &journal.Runs[i]
			if r.Run.ID == 0 {
				failures = append(failures, fmt.Errorf("pipeline %d: %s", r.PipelineID, r.Error))
				continue
			}
			if r.Run.State != "completed" {
				operation, cancel := context.WithTimeout(ctx, 30*time.Second)
				latest, err := client.GetPipelineRun(operation, journal.Project, r.Run.ID)
				cancel()
				if err != nil {
					return err
				}
				r.Run = latest
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d %s %s %s\n", r.Run.ID, r.Run.State, r.Run.Result, r.Run.WebURL)
			if r.Run.State != "completed" {
				pending = true
			} else if r.Run.Result != "succeeded" {
				failures = append(failures, fmt.Errorf("run %d: %s", r.Run.ID, r.Run.Result))
			}
		}
		if err := save(); err != nil {
			return err
		}
		if !pending {
			return errors.Join(failures...)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}
