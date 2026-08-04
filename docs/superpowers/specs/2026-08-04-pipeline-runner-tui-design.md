# Pipeline Runner TUI

## Objetivo

Transformar `azpipe` numa interface interativa para selecionar, validar, executar e acompanhar várias pipelines Azure DevOps, mantendo os comandos CLI existentes para automação.

## Experiência principal

Executar `azpipe` sem subcomandos abre a TUI. O fluxo tem quatro estados:

1. Contexto: organização e projeto, preenchidos a partir da configuração quando existirem.
2. Catálogo: lista única de pipelines, pesquisa e seleção múltipla.
3. Revisão: branch, modo, parâmetros e resultado da preview de cada pipeline.
4. Execução: runs lançadas em paralelo, estado final e ligação para Azure DevOps.

Os comandos existentes, como `azpipe pipelines list`, continuam inalterados.

## Catálogo

O catálogo mostra seleção, modo, ID, tipo e nome. O tipo é o primeiro segmento da pasta da definição. A pesquisa iniciada com `/` compara nome, ID, pasta, tipo, repositório e tags sem distinguir maiúsculas de minúsculas.

A linha ativa apresenta detalhe progressivo numa segunda linha discreta: repositório, pasta e tags. As restantes linhas permanecem compactas. Branch e parâmetros não aparecem no catálogo porque pertencem ao ecrã de revisão.

Teclas:

- `j/k` ou setas: navegar;
- `/`: pesquisar;
- `Space`: selecionar ou remover;
- `p`: alternar a pipeline entre `RUN` e `PLAN`;
- `b`: editar a branch global, inicialmente `main`;
- `Enter`: avançar para revisão;
- `q`: sair sem executar.

O modo `PLAN` envia o parâmetro YAML `planOnly=true`. A ferramenta não assume suporte apenas pelo nome da pipeline. O suporte é confirmado pela preview do Azure DevOps.

## Revisão e segurança

Ao entrar na revisão, o `azpipe` executa `previewRun=true` para todas as pipelines selecionadas, com concorrência limitada a quatro pedidos.

A revisão mostra por pipeline: ID e nome, branch efetiva, modo, parâmetros efetivos, estado da preview e erro devolvido pelo Azure DevOps.

Nenhuma pipeline pode ser lançada enquanto existir uma preview pendente ou falhada. Uma preview deteta parâmetros obrigatórios em falta, branch inválida e `planOnly` não suportado. `Esc` regressa ao catálogo sem perder a seleção.

Depois de todas as previews passarem, o operador escreve `EXECUTAR` para confirmar. Cancelamento, `q`, `Esc`, `Ctrl+C` ou EOF terminam sem criar runs.

## Execução e acompanhamento

As pipelines são lançadas em paralelo, com limite quatro. Uma falha ao lançar uma pipeline não cancela runs que o Azure DevOps já tenha aceite. A TUI mostra esse resultado parcial explicitamente e termina com código diferente de zero.

O ecrã final acompanha cada run até `completed` e apresenta estado, resultado e URL. `q` deixa de acompanhar, mas não cancela runs remotas.

## Arquitetura

- `internal/azdo`: acrescenta tags às definições e operações `PreviewPipeline`, `QueuePipeline` e `GetPipelineRun`.
- `internal/runner`: seleção, preparação de parâmetros, validação e coordenação concorrente, sem dependências visuais.
- `internal/tui/runner`: modelos Bubble Tea separados para contexto, catálogo, revisão e execução.
- `cmd`: abre a TUI apenas quando não existem subcomandos; acrescenta `azpipe demo` com dados locais e execução desativada.

O modo demo percorre catálogo e revisão com dados fictícios. Nunca constrói um cliente Azure DevOps e nunca apresenta uma ação capaz de lançar pipelines.

## Autenticação e configuração

O repositório público não inclui nomes, organizações, identidades, Keychain services ou paths reais de empresas.

Mantém-se a precedência atual para compatibilidade: ambiente, flags e configuração. O PAT persistido fica marcado como legado e o ficheiro é sempre gravado com permissão `0600`. A documentação recomenda `AZDO_PAT` ou um mecanismo externo de injeção de credenciais.

Uma futura distribuição privada pode disponibilizar um wrapper de autenticação específico, sem introduzir esse contrato no código público.

## Testes e validação

- testes de modelo Bubble Tea para navegação, pesquisa, seleção, modos e cancelamento;
- testes do gate que impede execução quando qualquer preview falha;
- testes da concorrência e resultados parciais;
- testes do cliente Azure DevOps com servidor HTTP local;
- teste de que `azpipe demo` não cria cliente nem chama rede;
- `go test -race ./...`, `go vet ./...` e build multiplataforma proporcional às alterações;
- demo manual no terminal após os testes automáticos.

## Fora do âmbito

- migração ou publicação do `infra-cleanup`;
- cópia para um repositório privado;
- publicação de nova release GitHub;
- armazenamento cross-platform em keyring;
- cancelamento remoto de runs;
- edição arbitrária de parâmetros YAML dentro da TUI.
