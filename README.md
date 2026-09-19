# Jungle Gaming — Processamento distribuído de apostas em Go

Serviço em Go + Uber Fx que movimenta carteiras de jogadores a partir de operações de provedores de jogos
(`BET`, `WIN`, `LOSS`, `REFUND`, `ROLLBACK`) recebidas por HTTP ou SQS, com idempotência persistente,
ledger append-only, inbox/outbox transacionais e autenticação OAuth 2.0 (Keycloak).

As decisões técnicas, garantias e limitações estão em [ARCHITECTURE.md](ARCHITECTURE.md).

## Pré-requisitos

| Ferramenta | Uso |
| --- | --- |
| Docker + Compose v2 | Aplicação, PostgreSQL, Keycloak, MiniStack (SQS) e migrations |
| Go 1.27.1 (mesma versão do `go.mod`) | Testes e execução fora do container |
| `make` (opcional) | Atalhos para os comandos abaixo |

## Subindo o ambiente

```sh
cp .env.example .env        # opcional: o compose tem defaults para todas as variáveis
docker compose up --build   # ou: make up
```

A ordem de inicialização é controlada por healthchecks: PostgreSQL → migrations (serviço `migrate`) →
Keycloak e MiniStack (filas provisionadas) → aplicação.

| Serviço | Endereço |
| --- | --- |
| API | http://localhost:8080 |
| Keycloak | http://localhost:8081 (admin/admin) |
| MiniStack (SQS) | http://localhost:4566 |
| PostgreSQL | localhost:5433 |

```sh
curl localhost:8080/health/live
curl localhost:8080/health/ready   # {"status":"ready","checks":{"postgres":"ok","sqs":"ok"}}
```

Três instâncias independentes (portas 8080, 8082 e 8083, mesmo banco e mesmas filas):

```sh
docker compose --profile multi up --build   # ou: make up_multi
```

## Variáveis de ambiente

Todas estão em [.env.example](.env.example), com valores locais de exemplo (sem segredos reais).

| Variável | Descrição |
| --- | --- |
| `HTTP_PORT` | Porta HTTP da aplicação (padrão 8080) |
| `DATABASE_URL` | Conexão PostgreSQL da aplicação (role `jungle_app`) |
| `APP_DB_PASSWORD` | Senha do role `jungle_app` |
| `TEST_DATABASE_URL` | Banco usado pelos testes de integração |
| `KEYCLOAK_ISSUER_URL` | Issuer (`iss`) aceito nos tokens |
| `KEYCLOAK_JWKS_URL` | Endpoint das chaves de assinatura |
| `KEYCLOAK_AUDIENCE` | Audience (`aud`) exigida nos tokens |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` / `AWS_REGION` | Credenciais SQS (MiniStack aceita `test`) |
| `SQS_ENDPOINT` | Endpoint SQS |
| `SQS_WAGER_QUEUE_URL` / `SQS_WAGER_DLQ_URL` | Fila de entrada e sua DLQ |
| `SQS_EVENT_QUEUE_URL` | Fila de eventos de integração publicados pela outbox |
| `SQS_ALLOWED_PROVIDERS` | Provedores aceitos em mensagens SQS (separados por vírgula) |
| `REFERENCE_RETRY_INITIAL_DELAY` / `REFERENCE_RETRY_MAX_DELAY` / `REFERENCE_TTL` | Backoff e TTL das referências pendentes |

## Filas

Provisionadas automaticamente pelo script [docker/ministack/init-sqs.sh](docker/ministack/init-sqs.sh)
quando o MiniStack sobe (idempotente):

| Fila | Papel |
| --- | --- |
| `wager-transactions.fifo` | Entrada `WagerTransactionRequested`, redrive para a DLQ após 10 recebimentos |
| `wager-transactions-dlq.fifo` | Mensagens com erro permanente ou tentativas esgotadas |
| `integration-events.fifo` | Eventos publicados pela outbox |
| `integration-events-dlq.fifo` | DLQ dos consumidores dos eventos |

Para reexecutar manualmente:

```sh
docker exec junglegaming-ministack bash /docker-entrypoint-initaws.d/ready.d/01-init-sqs.sh
```

## Migrations

Aplicadas automaticamente pelo serviço `migrate` do compose. Manualmente (sem instalar o CLI):

```sh
make migrate_up      # aplica todas as pendentes
make migrate_down    # reverte a última
```

Equivalente com o CLI [golang-migrate](https://github.com/golang-migrate/migrate), usando o dono do schema
(a aplicação conecta com o role `jungle_app`, que não tem permissão de DDL):

```sh
migrate -path migrations -database "postgres://postgres:postgres@localhost:5433/junglegaming?sslmode=disable" up
migrate -path migrations -database "postgres://postgres:postgres@localhost:5433/junglegaming?sslmode=disable" down 1
```

O login do role `jungle_app` é criado por [docker/postgres/01-app-role.sh](docker/postgres/01-app-role.sh) na primeira
subida do volume. Para um volume criado antes desse script: `make db_app_role`.

## Autenticação

O realm `junglegaming` é importado automaticamente de [keycloak/junglegaming-realm.json](keycloak/junglegaming-realm.json)
com os clients `client_credentials` usados como identidades de teste:

| Client | Papel | Acesso |
| --- | --- | --- |
| `provider-a` / `provider-a-secret` | `provider`, claim `provider_id=provider-a` | Operações e consultas de wager do próprio provedor |
| `provider-b` / `provider-b-secret` | `provider`, claim `provider_id=provider-b` | Idem, isolado do provider-a |
| `internal-service` / `internal-service-secret` | `internal` | Operações de carteira |
| `provider-a-short-lived` / `provider-a-short-lived-secret` | igual ao provider-a, token de 1 s | Teste de token expirado |

```sh
token() {
  curl -s -X POST localhost:8081/realms/junglegaming/protocol/openid-connect/token \
    -d grant_type=client_credentials -d client_id="$1" -d client_secret="$2" | jq -r .access_token
}

INTERNAL=$(token internal-service internal-service-secret)
PROVIDER_A=$(token provider-a provider-a-secret)
```

## Exemplos de chamadas

```sh
# Abrir carteira (internal)
curl -s -X POST localhost:8080/wallets \
  -H "Authorization: Bearer $INTERNAL" -H 'Content-Type: application/json' \
  -d '{"playerId":"0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1","initialBalance":{"amount":"1000.00","currency":"BRL"}}'
# {"id":"<walletId>","playerId":"0192f28f-...","balance":{"amount":"1000.00","currency":"BRL"},"version":1}

# Apostar (provider-a). Repetir a mesma chamada devolve o mesmo resultado com idempotentReplay=true.
curl -s -X POST localhost:8080/wagering/transactions \
  -H "Authorization: Bearer $PROVIDER_A" -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: provider-a:transaction-123' \
  -d '{"providerId":"provider-a","externalTransactionId":"transaction-123",
       "playerId":"0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1","walletId":"<walletId>",
       "roundId":"round-987","gameId":"fortune-chimp","kind":"BET",
       "money":{"amount":"25.00","currency":"BRL"}}'
# {"transactionId":"...","status":"PROCESSED","balance":{"amount":"975.00","currency":"BRL"},"idempotentReplay":false}

# Reversões usam referenceExternalTransactionId
#   "kind":"REFUND","referenceExternalTransactionId":"transaction-123"

# Consultas
curl -s localhost:8080/wagering/transactions/<transactionId> -H "Authorization: Bearer $PROVIDER_A"
curl -s localhost:8080/providers/provider-a/wagering/transactions/transaction-123 -H "Authorization: Bearer $PROVIDER_A"
curl -s localhost:8080/wallets/<walletId> -H "Authorization: Bearer $INTERNAL"
curl -s "localhost:8080/wallets/<walletId>/ledger?limit=50" -H "Authorization: Bearer $INTERNAL"
curl -s -X POST localhost:8080/wallets/<walletId>/reconciliation -H "Authorization: Bearer $INTERNAL"

# Métricas (Prometheus)
curl -s localhost:8080/metrics
```

Envio pela fila (o `messageId` do envelope é a identidade durável na inbox):

```sh
docker exec junglegaming-ministack aws --endpoint-url=http://localhost:4566 --region us-east-1 \
  sqs send-message \
  --queue-url http://localhost:4566/000000000000/wager-transactions.fifo \
  --message-group-id <walletId> --message-deduplication-id msg-123 \
  --message-body '{"messageId":"msg-123","type":"WagerTransactionRequested","occurredAt":"2026-09-08T12:00:00.000Z",
    "data":{"providerId":"provider-a","externalTransactionId":"transaction-124","idempotencyKey":"provider-a:transaction-124",
    "playerId":"0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1","walletId":"<walletId>","roundId":"round-987",
    "gameId":"fortune-chimp","kind":"BET","money":{"amount":"25.00","currency":"BRL"}}}'
```

Os códigos HTTP e corpos de erro estão documentados em [ARCHITECTURE.md](ARCHITECTURE.md#14-contrato-http).

## Testes

```sh
go test ./...          # unitários
go test -race ./...
go vet ./...           # ou: make vet (inclui os pacotes com build tags)
```

Testes que dependem de infraestrutura real usam build tags:

| Suíte | Tag | Pré-requisito | Comando |
| --- | --- | --- | --- |
| Integração (PostgreSQL + MiniStack + Fx) | `integration` | `make test_deps_up` | `make test_integration` |
| E2E HTTP com Keycloak real | `e2e` | `make up` | `make test_e2e` |
| Três instâncias independentes | `e2e multiprocess` | `make up_multi` | `make test_e2e_multi` |
| Reinício da aplicação | `e2e restart` | `make up` | `make test_e2e_restart` |

`make test_deps_up` sobe o banco de teste (porta 5434), Keycloak e MiniStack e aplica as migrations no banco de teste.
Os testes de integração leem `TEST_DATABASE_URL`, `SQS_*` e `AWS_*` do `.env`.

`make test_migrations` aplica todas as migrations, reverte todas e reaplica num banco descartável.

### Simulações de falha

| Cenário | Coberto por |
| --- | --- |
| 50 apostas iguais em paralelo, disputa 100.00 vs 2 × 80.00, carteiras em paralelo | `TestE2ESameBetFiftyTimesMovesMoneyOnce`, `TestCompetingBetsRepeatedly`, `TestConcurrentBetsOnSameWalletNeverFail`, `TestWagerServiceDoesNotGloballyLockWallets` |
| Mesma operação por HTTP e SQS | `TestHTTPAndSQSDeliverSameOperationOnce` |
| Consumidor interrompido após o commit e antes do delete | `TestSQSRedeliveryAfterCommitMovesMoneyOnce` |
| PostgreSQL indisponível durante o consumo | `TestSQSConsumerDelaysTransientFailure` |
| Erro permanente de mensagem | `TestSQSConsumerMovesPermanentFailureToDLQ` |
| Publishers concorrentes e recuperação da outbox | `TestOutboxDispatchersShareWorkWithoutDuplicates`, `TestOutboxRecoversAbandonedClaim` |
| Reversão antes da referência, resolução e expiração | `TestPendingReferencesResolveOrRejectWithoutBlocking`, `TestPendingReferenceExpiresAsReferenceNotFound` |
| Reinício preservando idempotência | `TestE2ERestartPreservesIdempotency` |
| Referência pendente retomada por outra instância | `TestPendingReferenceResumesOnAnotherInstance` |
| Token ausente, adulterado, expirado (Keycloak real) e validações do token | `TestHTTPRejectsMissingAuthentication`, `TestE2ERejectsTamperedJWT`, `TestE2ERejectsExpiredToken`, `TestVerifierRejectsInvalidTokens` |
| Isolamento entre provedores, inclusive em replays | `TestE2EProviderCannotReadAnotherProviderTransaction`, `TestE2EProvidersAreIsolatedOnReplays` |
| Ledger imutável e role da aplicação sem privilégios de reescrita | `TestWalletLedgerDatabaseBlocksUpdate`, `TestApplicationRoleCannotRewriteHistory`, `TestDatabaseRejectsChangesToTerminalWager` |

Manualmente, com a stack rodando:

```sh
docker compose pause postgres     # readiness 503, HTTP 503 com Retry-After, SQS em backoff
docker compose unpause postgres   # tudo retoma sem intervenção
docker compose kill -s SIGKILL app && docker compose up -d app   # queda abrupta; pendências retomadas
```

## Interface de apoio (`web/`)

Ferramenta de desenvolvimento em SvelteKit + Tailwind para exercitar e observar o serviço.
Não faz parte do desafio e não altera a arquitetura: nenhuma rota nova foi criada na API.

```sh
make up                      # a stack precisa estar no ar
cd web && npm install && npm run dev   # http://localhost:5173
```

Lê o `.env` da raiz, então usa os mesmos clients do Keycloak, filas e banco.

| Área | O que faz | Como acessa |
| --- | --- | --- |
| Providers | Emula um provider: obtém token `client_credentials`, cria ou carrega carteiras, envia BET/WIN/LOSS/REFUND/ROLLBACK com `Idempotency-Key` e mostra a resposta com o efeito (saldo, versão, transação e lançamento do ledger) | Só pelo fluxo real: Keycloak → API Go. Nunca acessa o banco |
| Admin | Cards (wallets, wagers, pending references, outbox pendente, SQS ready/in flight, DLQ), tabelas de transações, carteiras, ledger, outbox e inbox, e a transação aberta com suas relações | Lê o PostgreSQL em sessão somente leitura pelos arquivos `.server.ts` e as filas com `GetQueueAttributes` |

Os secrets ficam no servidor: o browser recebe apenas as claims do token.

## Estrutura

```text
cmd/server                 ponto de entrada (Fx)
internal/domain            Money, Wallet, WagerTransaction, WalletLedgerEntry (sem dependências externas)
internal/application       casos de uso, portas, eventos, códigos de falha
internal/infrastructure    postgres (pgx), sqs, keycloak
internal/transport/http    handlers Fiber, autenticação, contrato de erros
internal/worker            loop dos workers (consumer SQS, outbox, referências pendentes)
internal/observability     logger JSON e métricas
internal/wiring            composição Fx
migrations                 SQL versionado (up/down)
tests/integration, tests/e2e
web/                       interface de apoio (SvelteKit), fora do escopo do desafio
```
