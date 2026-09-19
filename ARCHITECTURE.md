# ARCHITECTURE

Serviço que processa operações financeiras de provedores de jogos (`BET`, `WIN`, `LOSS`, `REFUND`, `ROLLBACK`)
recebidas por HTTP ou SQS. O PostgreSQL é a fonte de verdade para saldo, idempotência, concorrência e
entrega de eventos: qualquer número de instâncias pode rodar ao mesmo tempo sem coordenação em memória.

Instruções de execução e testes estão no [README](README.md).

## 1. Camadas

```text
transport/http ─┐
infrastructure/sqs (consumer) ─┼──> application ──> domain
worker (loops) ─┘        │
                         └──> ports <── infrastructure (postgres, sqs publisher, keycloak)
```

| Pacote | Responsabilidade |
| --- | --- |
| `internal/domain` | `Money`, `Currency`, `Wallet`, `WagerTransaction`, `WalletLedgerEntry`. Sem dependência de Fx, HTTP, SQS ou pgx. |
| `internal/application` | Casos de uso, portas (interfaces), eventos tipados, códigos de falha, hash de idempotência. |
| `internal/infrastructure/postgres` | Repositórios pgx com SQL explícito, `TransactionManager`, relógio do banco. |
| `internal/infrastructure/sqs` | Consumer da fila de entrada, publisher da outbox, health check. |
| `internal/infrastructure/keycloak` | Validação de tokens OIDC. |
| `internal/transport/http` | Handlers Fiber, autenticação/autorização, contrato de erros, correlation id. |
| `internal/worker` | Loop genérico dos workers com shutdown gracioso. |
| `internal/observability` | Logger JSON com contexto e métricas Prometheus. |
| `internal/wiring` | Composição Fx. |

## 2. Dinheiro

- `Money` é um value object imutável com `shopspring/decimal` e `Currency`. Nunca passa por `float32`/`float64`:
  a entrada JSON é string, a persistência é `NUMERIC(20,2)` enviada e lida como texto (`$1::text::numeric`, `balance::text`).
- Entrada externa aceita apenas `^(0|[1-9][0-9]{0,17})\.[0-9]{2}$`: sem sinal, sem notação científica, `NaN`,
  `Infinity`, espaços ou zeros à esquerda, e sempre com duas casas. Não há normalização nem arredondamento:
  `"10.5"` é rejeitado.
- Limite: `999999999999999999.99` (18 dígitos inteiros), o mesmo de `NUMERIC(20,2)`. Soma, subtração e negação
  validam o limite (overflow vira erro).
- Aritmética e comparação exigem a mesma moeda (`ErrCurrencyMismatch`). Valores negativos só existem em
  diferenças internas (reconciliação); saldo e lançamentos são `>= 0` no domínio e por `CHECK`.
- Moeda: três letras maiúsculas (`CHECK` no banco). Os cenários usam BRL.

## 3. Modelo e invariantes no banco

| Tabela | Garantias |
| --- | --- |
| `wallets` | `UNIQUE (player_id, currency)`, `CHECK balance >= 0`, trigger: identidade imutável e `version` sobe exatamente 1 quando e só quando o saldo muda. |
| `wager_transactions` | Unicidade `(provider_id, idempotency_key)` e `(provider_id, external_transaction_id)`; um `OPENING` por carteira; uma reversão bem-sucedida por referência; `CHECK`s por tipo, origem (`OPENING` interno sem metadados externos), status e resultado financeiro; trigger impede qualquer alteração de transação terminal e de campos de negócio. |
| `wallet_ledger_entries` | `UNIQUE (wallet_id, transaction_id)`, `CHECK balance_after = balance_before ± amount`, FK para a transação da mesma carteira e moeda, triggers bloqueiam `UPDATE`, `DELETE` e `TRUNCATE`. |
| `inbox_messages` | PK `(consumer_name, message_id)`. |
| `outbox_events` | Identidade e payload imutáveis (trigger); só metadados de entrega mudam. |

A aplicação conecta com o role `jungle_app` (migration 000014), que só tem `SELECT/INSERT/UPDATE` nas tabelas
operacionais e `SELECT/INSERT` no ledger: sem `DELETE`, sem `TRUNCATE` e sem poder desabilitar triggers.
As migrations rodam com o dono do schema.

## 4. Transações SQL

- Biblioteca: `pgx/v5` com SQL explícito. O `TransactionManager` guarda o `pgx.Tx` no `context`; os repositórios
  usam a transação quando existe. Chamadas aninhadas participam da transação externa.
- Isolamento `READ COMMITTED` com locks explícitos de linha. Deadlocks e falhas de serialização (`40P01`, `40001`)
  reexecutam a transação até 3 vezes; falhas de conexão, timeouts e esgotamento viram `application.ErrUnavailable`.
- Limites da transação:

| Caso de uso | Uma única transação contém |
| --- | --- |
| `WalletService.Create` | carteira, `OPENING`, lançamento de crédito, outbox (`WagerTransactionProcessed`, `WalletBalanceChanged`) |
| `WagerService.Process` (HTTP e SQS) | transação externa, lock da carteira, resolução da referência, saldo, ledger, outbox |
| Handler SQS | registro na inbox + `WagerService.Process` + conclusão da inbox |
| Worker de referências | lock da pendência + processamento + reagendamento ou finalização |

- Relógio: `SELECT clock_timestamp()` do PostgreSQL, lido **depois** do lock da carteira. Todas as instâncias
  compartilham o mesmo relógio e movimentações da mesma carteira nunca observam tempo retroativo.
- `lock_timeout=5s`, `statement_timeout=15s`, `idle_in_transaction_session_timeout=30s`: esperas longas viram 503.

## 5. Concorrência

- Lock pessimista por carteira: `SELECT ... FOR NO KEY UPDATE`. `FOR UPDATE` foi descartado porque conflita com o
  `FOR KEY SHARE` que as FKs de `wager_transactions` e do ledger tomam na carteira: em teste, 198 de 240 apostas
  concorrentes terminavam em deadlock. `FOR NO KEY UPDATE` serializa os escritores da mesma carteira sem esse conflito.
- O `UPDATE` do saldo exige `version = versão anterior` (defesa contra lost update mesmo sem lock) e o banco rejeita
  saldo negativo. Não existe lock global nem em memória: carteiras diferentes avançam em paralelo.
- Duplicatas concorrentes: a transação externa é inserida primeiro com `ON CONFLICT DO NOTHING`; a segunda requisição
  espera o commit da primeira no índice único e devolve o resultado persistido.
- Verificado com 3 instâncias independentes (`docker compose --profile multi`), cada uma com seu pool e memória.

## 6. Idempotência

- Chaves persistentes, por provedor: `(provider_id, idempotency_key)` e `(provider_id, external_transaction_id)`.
- Mesma chave e mesmo conteúdo: resultado persistido com `idempotentReplay: true`, incluindo o saldo observado no
  processamento original (`balance_after`), não o saldo atual.
- Mesma chave com conteúdo diferente: `409 IDEMPOTENCY_CONFLICT`. Mesmo `externalTransactionId` com outra chave:
  `409 EXTERNAL_TRANSACTION_CONFLICT`. A chave recebida nunca é substituída por uma calculada.
- Hash do payload: SHA-256 (hex) do JSON canônico — chaves ordenadas em todos os níveis, sem espaços, sem escape
  HTML, UTF-8 — com os campos `externalTransactionId`, `gameId`, `kind`, `money{amount,currency}`, `playerId`,
  `providerId`, `referenceExternalTransactionId` (só quando presente), `roundId`, `walletId`. A chave de idempotência
  e os metadados de transporte (headers, `messageId`, `occurredAt`) ficam de fora. Normalização: UUIDs na forma
  canônica minúscula; `money` já é canônico pelo parser estrito; demais strings como recebidas.
- HTTP e SQS montam o mesmo `ProcessWagerCommand`, então a mesma operação tem o mesmo hash nas duas entradas.
  No SQS a chave é `data.idempotencyKey`, com a deduplicação adicional da inbox.

## 7. Máquina de estados

```text
            ┌──────────────► PROCESSED
PENDING ────┼──────────────► REJECTED
   │  ▲     └──────────────► FAILED
   ▼  │
PENDING_REFERENCE ─────────► REJECTED | FAILED
```

- O domínio valida cada transição; transações terminais não mudam (domínio e trigger).
- `PENDING` só existe dentro da transação síncrona: nunca é confirmado sozinho, então não há `PENDING` órfão após
  uma queda. O único estado intermediário confirmado é `PENDING_REFERENCE`, retomado pelo worker em qualquer instância.
- `OPENING` é interno: criado já `PROCESSED` na abertura com saldo positivo; rejeitado se vier de HTTP ou SQS.

**Falha transitória x permanente**

| Tipo | Exemplos | Efeito |
| --- | --- | --- |
| Transitória | conexão, timeout, `lock_timeout`, deadlock após retries, PostgreSQL/SQS fora | nada é persistido; HTTP `503` + `Retry-After`; SQS backoff; worker tenta no próximo ciclo |
| Entrada corrigível | JSON inválido, valor fora do formato, carteira inexistente ou de outro jogador | nada é persistido; HTTP `400/422`; SQS vai para a DLQ |
| Regra de negócio | saldo insuficiente, referência divergente | `REJECTED` com `failureCode`, auditável e reproduzido em replays |
| Permanente inesperada | erro determinístico ao processar uma pendência | `FAILED` com `PROCESSING_FAILED`, para auditoria |

## 8. Códigos de falha (`failureCode`)

Estáveis e definitivos: uma transação `REJECTED`/`FAILED` nunca é reprocessada; o provedor precisa de outra operação.

| Código | Quando |
| --- | --- |
| `BET_INSUFFICIENT_FUNDS` | `BET` maior que o saldo |
| `REVERSAL_INSUFFICIENT_FUNDS` | `ROLLBACK` que precisaria debitar mais que o saldo |
| `REFERENCE_NOT_FOUND` | referência não processada até o TTL |
| `REFERENCE_NOT_PROCESSABLE` | referência terminou `REJECTED` ou `FAILED` |
| `REFERENCE_MISMATCH` | referência com outro jogador, carteira, moeda, rodada ou valor |
| `REFERENCE_TYPE_NOT_ALLOWED` | tipo de referência não permitido para a operação |
| `DUPLICATE_REVERSAL` | a referência já tem uma reversão bem-sucedida |
| `PROCESSING_FAILED` | falha permanente inesperada (`FAILED`) |

## 9. Operações, referências e reversões

| Tipo | Movimento | Regras |
| --- | --- | --- |
| `BET` | débito | valor > 0, saldo suficiente |
| `WIN` | crédito | valor > 0; referência opcional a uma `BET` da mesma rodada |
| `LOSS` | nenhum | valor exatamente `0.00`; sem ledger, sem mudança de versão; só `WagerTransactionProcessed` |
| `REFUND` | crédito | referência obrigatória a uma `BET` processada, mesmo valor |
| `ROLLBACK` | inverso da referência | referência obrigatória a `BET`, `WIN` ou `REFUND` processado, mesmo valor |

- A referência é buscada por `(providerId, referenceExternalTransactionId)`, então pertence ao mesmo provedor.
  Jogador, carteira, moeda e rodada precisam coincidir. Uma operação não pode referenciar a si mesma (`422`).
- **Uma reversão bem-sucedida por referência** (índice único parcial + verificação sob o lock da carteira).
  Combinações sobre a mesma `BET`: após um `REFUND`, um `ROLLBACK` da `BET` é `DUPLICATE_REVERSAL` (e vice-versa),
  impedindo devolver o mesmo débito duas vezes. Desfazer um `REFUND` é um `ROLLBACK` que referencia o próprio
  `REFUND` (débito); depois dele a `BET` continua com a reversão registrada e não recebe um novo `REFUND`.
- **Referência ainda indisponível** (inexistente ou ela mesma pendente): a operação é confirmada em
  `PENDING_REFERENCE` com o evento `WagerTransactionPendingReference`. O worker tenta de novo com backoff
  exponencial de 5 s até 5 min (`REFERENCE_RETRY_*`), usando `FOR UPDATE SKIP LOCKED` para dividir o trabalho entre
  instâncias. Todo o estado fica no banco, então sobrevive a reinícios. Ao atingir o TTL (`REFERENCE_TTL`, 24 h) a
  operação vira `REJECTED` `REFERENCE_NOT_FOUND` com `WagerTransactionRejected`. Se a referência terminou sem
  sucesso: `REFERENCE_NOT_PROCESSABLE`. Divergências viram rejeição imediata, nunca retry infinito.

## 10. Ledger e reconciliação

- Cada mudança de saldo gera exatamente um lançamento no mesmo commit. `LOSS` e rejeições não geram lançamentos.
- A paginação `GET /wallets/{id}/ledger` usa cursor opaco (base64 de `createdAt` + `id`), ordem estável
  `created_at DESC, id DESC` e limite de 1 a 100 (padrão 50).
- `POST /wallets/{id}/reconciliation` lê saldo armazenado e soma do ledger num único `SELECT`, portanto num
  snapshot consistente e sem lock. `difference = armazenado - reconstruído`. Divergências aparecem na resposta,
  em log `ERROR` e na métrica `reconciliation_divergences_total`. Nunca altera o saldo.

## 11. Inbox e SQS

- Filas FIFO provisionadas por `docker/ministack/init-sqs.sh`: `wager-transactions.fifo` com redrive para
  `wager-transactions-dlq.fifo` após 10 recebimentos, visibility 60 s, long polling 20 s.
- Produtores devem usar `MessageGroupId = walletId` (ordem por carteira, paralelismo entre carteiras) e
  `MessageDeduplicationId = messageId`. A deduplicação FIFO dura 5 minutos; a inbox cobre o resto.
- Identidade durável: `messageId` do envelope. O hash SHA-256 do corpo é conferido em reentregas: o mesmo
  `messageId` com outro corpo é conflito permanente.
- Inbox, alterações de domínio, ledger, outbox e conclusão da inbox compartilham a transação. Uma pendência de
  referência conclui a mensagem; o worker assume a continuidade.
- Resultado de cada mensagem:

| Resultado | Ação |
| --- | --- |
| Processada, rejeitada por regra de negócio ou duplicada | `DeleteMessage` após o commit |
| Falha transitória | `ChangeMessageVisibility` com backoff `5 s × 2^(recebimentos-1)` até 5 min; o resto do lote volta com o mesmo atraso e o consumer pausa (1 s a 30 s) |
| Falha permanente (JSON/envelope inválido, validação, conflito, provedor não permitido) | enviada à DLQ na hora com o atributo `failureReason` e removida da fila |
| 10 recebimentos esgotados | redrive do SQS para a DLQ (cerca de 20 min de indisponibilidade contínua) |

- Queda após o commit e antes do delete: a mensagem volta após o visibility, a inbox responde que já foi concluída
  e ela é removida sem novo movimento.

## 12. Outbox e eventos

- Eventos são gravados na mesma transação da mudança que os originou e publicados depois por um worker, em
  `integration-events.fifo`.
- Vários publishers disputam a tabela com `UPDATE ... FROM (SELECT ... FOR UPDATE SKIP LOCKED LIMIT 1)`, gravando
  `claimed_by` e `claimed_until` (lease de 30 s). Só o dono do claim marca `published_at`. Falhas reagendam com
  backoff de 5 s até 5 min. Um claim abandonado (queda entre publicação e confirmação) volta a ficar disponível
  quando o lease vence e é republicado com o **mesmo `eventId`**; eventos confirmados e nunca publicados (queda
  entre commit e publicação) são assumidos por qualquer instância.
- Publicação: `MessageGroupId = aggregateId`, `MessageDeduplicationId = eventId`. Entrega at-least-once: o consumidor
  de eventos deve deduplicar por `eventId`. Não há ordem garantida entre agregados diferentes; para saldo, use
  `walletVersion`.
- Tipos concretos por evento; tipo e versão definidos pelo construtor (`NewWalletBalanceChangedEvent` etc.).

```json
{
  "eventId": "0a4b...",
  "eventType": "WalletBalanceChanged",
  "aggregateId": "<walletId>",
  "correlationId": "<correlation id do request ou messageId>",
  "causationId": "<messageId, quando veio do SQS>",
  "occurredAt": "2026-09-19T18:36:20.284531Z",
  "version": 1,
  "data": {
    "walletId": "...", "playerId": "...", "transactionId": "...", "direction": "DEBIT",
    "money": {"amount": "25.00", "currency": "BRL"},
    "balanceBefore": {"amount": "1000.00", "currency": "BRL"},
    "balanceAfter": {"amount": "975.00", "currency": "BRL"},
    "walletVersion": 2
  }
}
```

| Evento | Agregado | `data` |
| --- | --- | --- |
| `WagerTransactionProcessed` | transação | ids, `kind`, `money`, `balanceBefore`, `balanceAfter`, `referenceTransactionId` |
| `WagerTransactionRejected` | transação | ids, `kind`, `money`, `failureCode`, `failureMessage` |
| `WalletBalanceChanged` | carteira | `walletId`, `transactionId`, `direction`, `money`, `balanceBefore`, `balanceAfter`, `walletVersion` |
| `WagerTransactionPendingReference` | transação | ids, `referenceExternalTransactionId`, `kind`, `money` |

## 13. Autenticação e autorização

- **IdP**: Keycloak 26.7 (recomendado pelo desafio, OIDC padrão, `client_credentials` para serviço a serviço).
  O realm, os clients e os service accounts são importados automaticamente com segredos vindos do ambiente.
- **Validação** (`go-oidc`): assinatura RS256 com chaves do JWKS (cache, download com timeout de 5 s), `iss`,
  `aud = junglegaming-api`, `exp`, `typ = Bearer` (tokens de ID e refresh são recusados) e `azp` obrigatório.
- **Modelo de permissões**:

| Papel | Identidade | Pode |
| --- | --- | --- |
| `provider` | claim `provider_id` fixada por client no Keycloak | enviar operações e consultar apenas as próprias transações |
| `internal` | client `internal-service` | abrir e consultar carteiras, ledger e reconciliação |

- O `providerId` autorizado vem sempre do token. `providerId` do corpo diferente do token: `403`. Transação de
  outro provedor: `404` (sem revelar existência). `/providers/{id}/...` com outro id: `403`. As chaves de
  idempotência são isoladas por provedor, inclusive em replays. Provedores não acessam rotas de carteira e o
  serviço interno não envia operações.
- **Mensageria**: no SQS o `providerId` vem do corpo, então o acesso é controlado pelo broker e revalidado no
  consumer. Em produção cada provedor teria só `sqs:SendMessage` na fila de entrada e a aplicação só
  `ReceiveMessage`, `DeleteMessage`, `ChangeMessageVisibility` e `GetQueueAttributes` nela, mais `SendMessage` na DLQ
  e na fila de eventos:

```json
{
  "Effect": "Allow",
  "Principal": {"AWS": "arn:aws:iam::<conta>:role/provider-a-publisher"},
  "Action": "sqs:SendMessage",
  "Resource": "arn:aws:sqs:<região>:<conta>:wager-transactions.fifo"
}
```

  O consumer rejeita (DLQ) qualquer `providerId` fora de `SQS_ALLOWED_PROVIDERS` e aplica todas as validações de
  domínio. O MiniStack não avalia políticas IAM; localmente, a proteção efetiva é a allowlist.

## 14. Contrato HTTP

Sucesso devolve o corpo do recurso, como nos exemplos do desafio. Erros usam `{"code","message","details"}`.

| Situação | Status | `code` |
| --- | --- | --- |
| Operação processada ou rejeitada por regra de negócio | `200` | — (`status` e `failureCode` no corpo) |
| Aguardando referência | `202` | — (`status: PENDING_REFERENCE`) |
| Carteira criada | `201` | — |
| JSON inválido, campo obrigatório ausente, `limit`/cursor inválido | `400` | `INVALID_PAYLOAD` |
| Header `Idempotency-Key` ausente | `400` | `IDEMPOTENCY_KEY_REQUIRED` |
| Token ausente, inválido ou expirado | `401` | `UNAUTHORIZED` |
| Papel ou provedor não autorizado | `403` | `FORBIDDEN` |
| Recurso inexistente ou de outro provedor | `404` | `NOT_FOUND` |
| Chave reutilizada com outro conteúdo | `409` | `IDEMPOTENCY_CONFLICT` |
| Transação externa recebida com outra chave | `409` | `EXTERNAL_TRANSACTION_CONFLICT` |
| Carteira duplicada para jogador e moeda | `409` | `WALLET_ALREADY_EXISTS` |
| Corpo acima de 16 KB | `413` | `PAYLOAD_TOO_LARGE` |
| Valor, moeda, tipo, ids ou referência inválidos (campo em `details`) | `422` | `VALIDATION_FAILED` |
| Dependência indisponível (repetir a mesma requisição) | `503` + `Retry-After` | `SERVICE_UNAVAILABLE` |
| Erro inesperado | `500` | `INTERNAL_ERROR` |

Rejeições de negócio usam `200` porque são o resultado definitivo e persistido da operação, reproduzido em replays;
`422` indica entrada corrigível que não foi registrada.

## 15. Uber Fx, ciclo de vida e shutdown

- Módulos (`config`, `observability`, `postgres`, `keycloak`, `sqs`, `worker`, `application`, `http`) só declaram
  construtores; `fx.Invoke` registra os workers e o servidor. `internal/wiring` valida o grafo em teste
  (`fx.ValidateApp`).
- Inicialização: a configuração é validada ao carregar (variável ausente ou inválida impede a subida); o pool faz
  `Ping` no `OnStart`; depois sobem os workers e, por último, o HTTP.
- Encerramento (`SIGTERM`, prazo de 20 s): o HTTP para de aceitar conexões e conclui as requisições; cada worker
  para de buscar trabalho e termina o item em andamento (o consumer devolve à fila, com visibility 0, as mensagens
  recebidas e não iniciadas); por fim o pool fecha. Se o prazo estourar, o trabalho em andamento é cancelado,
  a transação faz rollback e é retomada por outra instância.

## 16. Observabilidade

- Logs JSON (`slog`) com `correlationId`, `messageId`, `transactionId`, `walletId`, `providerId`/`clientId`
  quando disponíveis. O correlation id vem de `X-Correlation-Id` (ou é gerado), volta na resposta e segue para os
  eventos. Tokens, segredos, `DATABASE_URL` e payloads financeiros completos não são registrados.
- Métricas em `/metrics`: `wager_transactions_total{source,kind,status}`, `wager_idempotent_replays_total`,
  `wager_conflicts_total`, `wager_processing_duration_seconds`, `db_transaction_retries_total{sqlstate}`,
  `sqs_messages_total{result}`, `sqs_dlq_messages`, `outbox_publications_total`, `outbox_pending_events`,
  `outbox_oldest_pending_age_seconds`, `pending_reference_transactions`, `reconciliation_divergences_total`.
- `GET /health/live` (processo) e `GET /health/ready` (PostgreSQL e filas SQS).

## 17. Limitações, interpretações e trabalho não concluído

- O MiniStack não aplica IAM: as políticas de broker estão documentadas, mas localmente só a allowlist protege a fila.
- Um provedor pode operar qualquer carteira cujo `playerId` informe corretamente; não há vínculo provedor ↔ jogador
  porque o desafio não o define.
- Moedas são aceitas por formato (três letras) e todas usam duas casas decimais; não há tabela ISO 4217.
- Se a referência ainda estiver pendente quando o TTL vencer, o resultado também é `REFERENCE_NOT_FOUND`.
- Não há ordem garantida de publicação entre agregados diferentes.
- `outbox_events` e `inbox_messages` crescem sem limpeza (não há job de retenção).
- A abertura de carteira não é idempotente por chave; duplicidade por jogador e moeda retorna `409`, como pedido.
- Keycloak roda em `start-dev`; PostgreSQL 17 (a versão 18 muda o caminho do volume na imagem oficial).
- Não implementados (opcionais no desafio): ledger de partidas dobradas, tracing OpenTelemetry e testes de carga.
  Há benchmarks de `ParseMoney` e do hash de idempotência (`go test -bench .`).
