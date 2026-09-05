package azdo

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/build"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
	"io"
	"os"
	"strings"
)

func LoadContracts(path string) ([]PlanContract, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var contracts []PlanContract
	d := json.NewDecoder(io.LimitReader(f, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(&contracts); err != nil {
		return nil, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("contratos devem conter um único array JSON")
	}
	seen := map[string]bool{}
	for _, c := range contracts {
		key := strings.ToLower(c.Organization) + "/" + c.Project + fmt.Sprintf("/%d", c.PipelineID)
		if seen[key] {
			return nil, fmt.Errorf("contrato duplicado")
		}
		seen[key] = true
		if _, err := hex.DecodeString(c.Commit); err != nil {
			return nil, fmt.Errorf("SHA inválido")
		}
		if c.Organization == "" || c.Project == "" || c.PipelineID <= 0 || c.DefinitionVersion <= 0 || len(c.Commit) != 40 || c.Parameter == "" || c.Evidence == "" || c.PlanValue == c.RunValue {
			return nil, fmt.Errorf("contrato PLAN incompleto")
		}
		if c.Type != "boolean" && c.Type != "string" {
			return nil, fmt.Errorf("tipo PLAN não suportado: %s", c.Type)
		}
		if c.Type == "boolean" && !((c.PlanValue == "true" && c.RunValue == "false") || (c.PlanValue == "false" && c.RunValue == "true")) {
			return nil, fmt.Errorf("valores booleanos inválidos")
		}
	}
	return contracts, nil
}

func (c *azdoClient) PrepareRun(ctx context.Context, project string, request RunRequest) (RunRequest, error) {
	bc, err := build.NewClient(ctx, c.conn)
	if err != nil {
		return request, err
	}
	d, err := bc.GetDefinition(ctx, build.GetDefinitionArgs{Project: &project, DefinitionId: &request.PipelineID})
	if err != nil {
		return request, err
	}
	if d == nil || d.Repository == nil || derefStr(d.Repository.Type) != "TfsGit" || d.Revision == nil {
		return request, fmt.Errorf("versionamento requer definição Azure Repos Git")
	}
	gc, err := git.NewClient(ctx, c.conn)
	if err != nil {
		return request, err
	}
	ref := normalizeBranch(request.Branch)
	filter := strings.TrimPrefix(ref, "refs/")
	refs, err := gc.GetRefs(ctx, git.GetRefsArgs{Project: &project, RepositoryId: d.Repository.Id, Filter: &filter})
	if err != nil {
		return request, err
	}
	if refs == nil {
		return request, fmt.Errorf("resposta sem referências Git")
	}
	for _, r := range refs.Value {
		if derefStr(r.Name) == ref {
			request.Commit = derefStr(r.ObjectId)
			break
		}
	}
	if len(request.Commit) != 40 {
		return request, fmt.Errorf("branch sem SHA resolvido: %s", ref)
	}
	request.DefinitionVersion = *d.Revision
	for _, contract := range c.contracts {
		if contract.Project == project && contract.PipelineID == request.PipelineID && strings.TrimRight(contract.Organization, "/") == strings.TrimRight(c.conn.BaseUrl, "/") {
			if contract.Commit != request.Commit || contract.DefinitionVersion != request.DefinitionVersion {
				return request, fmt.Errorf("contrato RUN/PLAN desactualizado: rever SHA e versão da definição")
			}
		}
	}
	request.PreviewHash, err = c.previewHash(ctx, project, request)
	return request, err
}

func NewWithContracts(orgURL, pat string, contracts []PlanContract) Client {
	c := New(orgURL, pat).(*azdoClient)
	c.contracts = contracts
	return c
}
