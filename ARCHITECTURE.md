# ARCHITECTURE

## 1. Sobre o projeto

Este projeto implementa o backend do desafio técnico da Jungle Gaming.

O objetivo principal é processar operações financeiras de jogo com segurança, garantindo que uma mesma operação não movimente dinheiro duas vezes, que o saldo da carteira nunca fique negativo e que o sistema continue consistente mesmo com concorrência, retries, múltiplas instâncias ou falhas durante o processamento.

O serviço recebe operações por HTTP e SQS, mas ambos os transportes utilizam a mesma regra de negócio e a mesma camada de aplicação.

As principais operações financeiras são:

```text
OPENING
BET
WIN
LOSS
REFUND
ROLLBACK
```

A arquitetura é organizada em camadas simples:

```text
Transport / Workers
        |
        v
   Application
        |
        v
      Domain
        ^
        |
 Infrastructure
```

A regra central é:

> O domínio define o que é válido. A aplicação coordena o caso de uso. A infraestrutura conversa com banco, SQS e IdP.

PostgreSQL é a fonte final de verdade para consistência financeira, concorrência e idempotência.

---

## 2. Estrutura do projeto

```text
cmd/
└── server/
    └── main.go

internal/
├── config/
│
├── domain/
│   ├── errors.go
│   ├── currency.go
│   ├── money.go
│   ├── wallet.go
│   ├── wallet_ledger_entry.go
│   └── wager_transaction.go
│
├── application/
│   ├── ports.go
│   ├── commands.go
│   ├── wallet_service.go
│   ├── wager_service.go
│   ├── reconciliation_service.go
│   └── events.go
│
├── infrastructure/
│   ├── postgres/
│   ├── oidc/
│   └── sqs/
│
├── transport/
│   └── http/
│
├── workers/
│
├── observability/
│
└── wiring/

migrations/

tests/
└── integration/

Dockerfile
docker-compose.yml
.env.example
README.md
ARCHITECTURE.md
```

---

## 3. Responsabilidade de cada pasta

| Pasta | Responsabilidade |
| --- | --- |
| `cmd/server` | Ponto de entrada da aplicação. Inicializa o container Fx. |
| `internal/config` | Carrega e valida configuração e variáveis de ambiente. |
| `internal/domain` | Regras de negócio puras. Não conhece HTTP, SQS, PostgreSQL, Fx ou Keycloak. |
| `internal/application` | Coordena os casos de uso e define as interfaces necessárias para infraestrutura. |
| `internal/infrastructure/postgres` | Implementa repositories, transactions e queries com pgx. |
| `internal/infrastructure/oidc` | Valida identidade e autorização usando o IdP externo. |
| `internal/infrastructure/sqs` | Implementa consumo e publicação através de SQS. |
| `internal/transport/http` | Recebe requests HTTP, autentica, valida transporte e chama a application. |
| `internal/workers` | Executa outbox, retry de referência e consumidores assíncronos. |
| `internal/observability` | Logging estruturado, métricas e health checks. |
| `internal/wiring` | Faz a composição das dependências usando Uber Fx. |
| `migrations` | Define a estrutura e as garantias do banco através de SQL versionado. |
| `tests/integration` | Valida comportamento envolvendo PostgreSQL, Keycloak, SQS e múltiplas instâncias. |

### Dependências entre camadas

```text
HTTP Handler ----------------------+
                                   |
SQS Consumer ----------------------+----> Application ----> Domain
                                   |
Background Workers ----------------+

Infrastructure -------------------------> implements Application ports
```

O `domain` não importa nenhuma camada externa.

A `application` conhece apenas o domínio e interfaces.

A infraestrutura implementa essas interfaces.

---

# 4. Modelo de dados

O banco não é apenas persistência. Ele também protege as principais invariantes financeiras do sistema.

As tabelas centrais são:

```text
wallets
wager_transactions
wallet_ledger_entries
inbox
outbox
```

---

## 4.1 Wallets

Representa o saldo atual de um jogador para uma moeda.

### Estrutura

```text
wallets
├── id                  UUID
├── player_id           UUID
├── currency            VARCHAR(3)
├── balance             NUMERIC(20,2)
├── version             BIGINT
├── created_at          TIMESTAMPTZ
└── updated_at          TIMESTAMPTZ
```

### Responsabilidade

A tabela mantém o estado financeiro atual da carteira.

Regras principais:

```text
(player_id, currency) é único
balance >= 0
version >= 1
```

A versão começa em `1` e só aumenta quando o saldo muda.

Uma carteira nunca depende de lock em memória. Concorrência financeira é protegida pelo PostgreSQL através de lock por linha.

---

## 4.2 Wager Transactions

Representa cada operação financeira recebida ou criada internamente.

### Estrutura

```text
wager_transactions
├── id
├── provider_id
├── external_transaction_id
├── idempotency_key
├── payload_hash
│
├── wallet_id
├── player_id
├── round_id
├── game_id
│
├── type
├── amount
├── currency
│
├── reference_external_transaction_id
├── reference_transaction_id
├── reference_transaction_type
│
├── status
├── failure_code
├── failure_message
│
├── balance_before
├── balance_after
│
├── reference_attempts
├── next_reference_attempt_at
├── reference_expires_at
│
├── created_at
├── updated_at
└── completed_at
```

### Tipos

```text
OPENING
BET
WIN
LOSS
REFUND
ROLLBACK
```

### Status

```text
PENDING
PENDING_REFERENCE
PROCESSED
REJECTED
FAILED
```

### Responsabilidade

A tabela registra a operação financeira e seu resultado final.

Ela também fornece a base para:

```text
idempotência
replay
rastreamento
referências
reversões
retry de referência
falhas auditáveis
```

Identidades externas são protegidas por unicidade:

```text
(provider_id, external_transaction_id)
(provider_id, idempotency_key)
```

Uma operação terminal não pode ser processada novamente.

---

## 4.3 Wallet Ledger Entries

Representa cada alteração real de saldo.

### Estrutura

```text
wallet_ledger_entries
├── id
├── wallet_id
├── transaction_id
├── direction
├── amount
├── currency
├── balance_before
├── balance_after
└── created_at
```

### Direções

```text
DEBIT
CREDIT
```

### Responsabilidade

O ledger fornece o histórico financeiro imutável da carteira.

Cada linha prova uma alteração de saldo:

```text
DEBIT
balance_after = balance_before - amount

CREDIT
balance_after = balance_before + amount
```

Uma operação que não altera saldo não cria ledger.

Exemplo:

```text
LOSS
amount = 0.00
wallet permanece igual
nenhuma entrada no ledger
```

O banco bloqueia `UPDATE`, `DELETE` e `TRUNCATE` do ledger durante a execução normal da aplicação.

---

## 4.4 Inbox

A inbox garante deduplicação durável das mensagens SQS.

### Estrutura

```text
inbox
├── consumer_name
├── message_id
├── message_hash
├── received_at
└── completed_at
```

### Responsabilidade

A chave lógica é:

```text
(consumer_name, message_id)
```

Quando uma mensagem SQS já foi concluída, um novo recebimento não executa novamente a operação financeira.

O registro da inbox é concluído dentro da mesma transação SQL da operação financeira.

---

## 4.5 Outbox

A outbox garante que eventos derivados de uma transação confirmada não sejam perdidos.

### Estrutura

```text
outbox
├── event_id
├── event_type
├── aggregate_id
├── correlation_id
├── causation_id
├── version
├── payload
├── occurred_at
│
├── attempts
├── next_attempt_at
├── claimed_by
├── claimed_until
├── published_at
└── created_at
```

### Responsabilidade

Os eventos são inseridos na outbox dentro da mesma transação SQL da mudança financeira.

Depois do commit, um worker publica esses eventos.

```text
financial transaction
        |
        +--> wallet
        +--> wager
        +--> ledger
        +--> outbox
        |
      COMMIT
        |
        v
 outbox publisher
        |
        v
       SQS
```

Isso evita o cenário:

```text
saldo confirmado
+
evento perdido
```

---

## 4.6 Relacionamentos principais

```text
Player
  |
  v
Wallet
  |
  +---------------------------+
  |                           |
  v                           v
WagerTransaction ------> WalletLedgerEntry
        |
        |
        +------ reference ------> WagerTransaction
```

Uma `WagerTransaction` pertence a uma wallet.

Uma entrada de ledger pertence à wallet e à transaction que gerou aquela movimentação.

REFUND e ROLLBACK podem referenciar outra transaction.

---

# 5. Fluxo da aplicação

## 5.1 Visão geral

```text
                   +----------------+
HTTP ------------->|                |
                   |  Application   |-----> Domain
SQS -------------->|                |
                   +-------+--------+
                           |
                           v
                     PostgreSQL
                           |
              +------------+------------+
              |                         |
              v                         v
            Inbox                     Outbox
                                        |
                                        v
                                   Outbox Worker
                                        |
                                        v
                                       SQS
```

HTTP e SQS nunca implementam regras financeiras diferentes.

Os dois caminhos chegam ao mesmo serviço de aplicação.

---

# 6. Criação de wallet

## 6.1 Wallet com saldo inicial zero

```text
Request
   |
   v
Application
   |
   v
BEGIN
   |
   +--> INSERT wallet
   |       balance = 0.00
   |       version = 1
   |
 COMMIT
```

Resultado:

```text
wallet criada
nenhum OPENING
nenhum ledger
nenhum evento financeiro
```

---

## 6.2 Wallet com saldo inicial positivo

Exemplo:

```text
initialBalance = 100.00 BRL
```

Fluxo:

```text
BEGIN
  |
  +--> INSERT wallet
  |       balance = 100.00
  |       version = 1
  |
  +--> INSERT wager OPENING / PROCESSED
  |
  +--> INSERT ledger CREDIT
  |       0.00 -> 100.00
  |
  +--> INSERT outbox
          WagerTransactionProcessed
          WalletBalanceChanged
  |
COMMIT
```

A wallet já nasce com o saldo inicial.

Por isso o `OPENING` não incrementa a versão para `2`.

---

# 7. Processamento de wager

O caso de uso principal será responsável por processar:

```text
BET
WIN
LOSS
REFUND
ROLLBACK
```

Fluxo simplificado:

```text
Request / SQS Message
        |
        v
Authentication
        |
        v
Application ProcessWager
        |
        v
Persistent Idempotency Check
        |
        +------ replay ------> return persisted result
        |
        v
Resolve reference if necessary
        |
        +------ missing ------> PENDING_REFERENCE
        |
        v
Lock Wallet
SELECT ... FOR UPDATE
        |
        v
Apply Domain Rule
        |
        v
Wallet Debit / Credit / None
        |
        +------ balance changed ------> Ledger Entry
        |
        v
Persist Wager Result
        |
        v
Write Outbox Events
        |
        v
COMMIT
```

---

## 7.1 BET

```text
BET 25.00
Wallet 100.00

        BET
         |
         v
      DEBIT
         |
         v
Wallet.Debit(25.00)
         |
         v
100.00 ------> 75.00
         |
         +--> Ledger DEBIT
         +--> Wager PROCESSED
         +--> WalletBalanceChanged
         +--> WagerTransactionProcessed
```

Se o saldo for insuficiente:

```text
wallet não muda
nenhum ledger
wager = REJECTED
failureCode = BET_INSUFFICIENT_FUNDS
```

---

## 7.2 WIN

```text
WIN 25.00
Wallet 100.00

        WIN
         |
         v
      CREDIT
         |
         v
Wallet.Credit(25.00)
         |
         v
100.00 ------> 125.00
         |
         +--> Ledger CREDIT
         +--> Wager PROCESSED
         +--> Events
```

WIN pode opcionalmente referenciar um BET da mesma rodada.

---

## 7.3 LOSS

LOSS possui valor exato:

```text
0.00
```

Fluxo:

```text
LOSS
 |
 v
NO WALLET MOVEMENT
 |
 +--> wallet não muda
 +--> version não muda
 +--> nenhum ledger
 +--> Wager PROCESSED
 +--> WagerTransactionProcessed
```

Não existe `WalletBalanceChanged` para LOSS.

---

## 7.4 REFUND

REFUND devolve integralmente um BET processado.

```text
BET 25.00
100.00 -> 75.00

REFUND 25.00
75.00 -> 100.00
```

Fluxo:

```text
REFUND
   |
   v
resolve referenced BET
   |
   v
validate
provider
player
wallet
currency
round
amount
   |
   v
CREDIT wallet
   |
   +--> Ledger CREDIT
   +--> Wager PROCESSED
   +--> Events
```

Refund parcial não é permitido.

---

## 7.5 ROLLBACK

ROLLBACK executa o movimento oposto da operação referenciada.

```text
ROLLBACK(BET)
BET foi DEBIT
rollback = CREDIT

ROLLBACK(WIN)
WIN foi CREDIT
rollback = DEBIT

ROLLBACK(REFUND)
REFUND foi CREDIT
rollback = DEBIT
```

Representação:

```text
              Reference
                  |
       +----------+----------+
       |          |          |
      BET        WIN       REFUND
       |          |          |
       v          v          v
    CREDIT      DEBIT      DEBIT
       \          |          /
        +---------+---------+
                  |
                  v
               Wallet
```

Se um rollback que precisa debitar não tiver saldo suficiente:

```text
wallet não muda
nenhum ledger
wager = REJECTED
failureCode = REVERSAL_INSUFFICIENT_FUNDS
```

---

# 8. Referências e PENDING_REFERENCE

REFUND e ROLLBACK exigem referência.

WIN pode receber referência opcional.

A busca utiliza:

```text
(providerId, referenceExternalTransactionId)
```

Se a referência ainda não existe:

```text
Wager arrives
    |
    v
Reference lookup
    |
    +------ found ------> normal processing
    |
    v
not found
    |
    v
PENDING_REFERENCE
    |
    +--> attempts
    +--> next attempt
    +--> expiration
    |
    v
Pending Reference Worker
```

Worker:

```text
PENDING_REFERENCE
       |
       +---- reference found ----> PENDING ----> process
       |
       +---- not found ----------> retry later
       |
       +---- TTL exhausted ------> REJECTED
                                   REFERENCE_NOT_FOUND
```

O retry é persistido no PostgreSQL e sobrevive a restart da aplicação.

---

# 9. Idempotência e replay

Antes de movimentar a wallet, a application verifica a identidade persistida da operação.

Proteções:

```text
(providerId, idempotencyKey)
(providerId, externalTransactionId)
```

Cada request também possui um hash determinístico do payload financeiro.

### Mesmo idempotency key + mesmo payload

```text
original request
       |
       v
already exists
       |
       v
return persisted result
idempotentReplay = true
```

Nenhum dinheiro é movimentado novamente.

### Mesmo idempotency key + payload diferente

```text
CONFLICT
```

### Mesmo external transaction ID com outra key

```text
CONFLICT
```

Um replay de uma operação concluída devolve o `balanceAfter` originalmente observado naquela operação, não o saldo atual da wallet.

---

# 10. Concorrência

Concorrência é protegida por wallet usando PostgreSQL.

```sql
SELECT ...
FROM wallets
WHERE id = $1
FOR UPDATE;
```

Exemplo:

```text
Wallet = 100.00

Instance A                  Instance B
    |                           |
BET 80                      BET 80
    |                           |
FOR UPDATE --------------------> waits
    |
100 -> 20
COMMIT
                                |
                                v
                           reads 20
                                |
                         insufficient
                                |
                           REJECTED
```

Resultado:

```text
1 BET PROCESSED
1 BET REJECTED
wallet = 20.00
1 ledger entry
```

Não existe mutex global ou lock em memória.

Assim a garantia continua válida com várias instâncias da aplicação.

---

# 11. Fluxo SQS

SQS utiliza o mesmo caso de uso financeiro do HTTP.

A diferença é a inbox.

```text
SQS message
    |
    v
BEGIN
    |
    +--> Inbox dedupe
    |
    +--> Process wager
    |
    +--> wallet / wager / ledger
    |
    +--> outbox
    |
    +--> complete inbox
    |
COMMIT
    |
    v
Delete SQS message
```

Se a aplicação morrer depois do commit e antes do delete:

```text
SQS redelivers
      |
      v
Inbox says completed
      |
      v
no financial replay
      |
      v
delete message
```

---

# 12. Rastreamento financeiro

Existem três níveis de rastreamento.

### Estado atual

```text
wallets
```

Mostra o saldo atual da carteira.

### Operação de negócio

```text
wager_transactions
```

Mostra:

```text
quem enviou
qual operação
id externo
idempotency key
status
referência
falha
saldo observado
```

### Histórico financeiro

```text
wallet_ledger_entries
```

Mostra cada débito e crédito efetivamente realizado.

Fluxo de auditoria:

```text
Wallet
  |
  v
Wager Transaction
  |
  v
Ledger Entry
  |
  v
before / amount / after
```

---

# 13. Reconciliação

A reconciliação verifica se o saldo atual da wallet é compatível com o histórico do ledger.

```text
Ledger
  |
  v
rebuild expected balance
  |
  v
compare
  |
  +-----------------------+
  |                       |
  v                       v
stored balance       calculated balance
  |                       |
  +-----------+-----------+
              |
              v
          difference
```

A reconciliação é somente leitura.

Ela não corrige automaticamente a carteira.

Se existir divergência, ela é retornada e registrada para observabilidade.

---

# 14. Eventos

Eventos financeiros são registrados pela application na outbox.

Principais eventos:

```text
WagerTransactionProcessed
WagerTransactionRejected
WalletBalanceChanged
WagerTransactionPendingReference
```

Fluxo:

```text
Application
    |
    v
SQL Transaction
    |
    +--> business changes
    +--> outbox event
    |
  COMMIT
    |
    v
Outbox Worker
    |
    v
Publish
```

Os eventos possuem `eventId` estável.

Se o worker publicar e morrer antes de marcar `published_at`, o evento pode ser publicado novamente com o mesmo `eventId`.

Isso é entrega at-least-once, e não perda de evento.

---

# 15. Serviços externos

## 15.1 PostgreSQL

PostgreSQL é responsável por:

```text
persistência
transações
row locking
constraints
idempotência
ledger
inbox
outbox
pending references
```

As operações financeiras utilizam `pgx` com SQL explícito.

O banco é a garantia final contra corrida entre múltiplas instâncias.

---

## 15.2 Keycloak

Keycloak funciona como Identity Provider OAuth2/OIDC.

O fluxo principal é `client_credentials`.

```text
Provider Service
      |
      v
   Keycloak
      |
      v
 Access Token
      |
      v
Jungle Backend
```

O `providerId` utilizado pela aplicação vem da identidade autenticada.

Um provider não pode consultar ou reutilizar transações pertencentes a outro provider.

Operações internas de wallet utilizam autorização de serviço interno.

---

## 15.3 AWS SQS / LocalStack

Durante desenvolvimento e integração, SQS é executado através do LocalStack.

Filas esperadas:

```text
wager-transactions.fifo
wager-transactions-dlq.fifo
```

Responsabilidades:

```text
entrada assíncrona de wagers
retry de mensagem
DLQ
publicação de eventos
```

FIFO auxilia a ordenação, mas não substitui idempotência persistente.

---

# 16. Runtime da aplicação

## Uber Fx

Uber Fx cuida apenas de composição e lifecycle.

```text
Config
  |
  +--> PostgreSQL
  +--> Keycloak
  +--> SQS
  +--> Application Services
  +--> HTTP
  +--> Workers
```

O domínio não conhece Fx.

## Fiber

Fiber é responsável apenas pela camada HTTP:

```text
route
request parsing
auth middleware
response mapping
```

Nenhuma regra financeira deve ficar no handler.

---

# 17. Observabilidade

Logs estruturados incluem, quando disponíveis:

```text
correlationId
messageId
transactionId
walletId
providerId
```

Health endpoints:

```text
GET /health/live
GET /health/ready
```

Readiness verifica dependências necessárias como PostgreSQL e SQS.

Métricas relevantes incluem:

```text
wagers por status
retries
idempotency conflicts
DLQ
outbox delay
processing latency
reconciliation divergence
```

---

# 18. Garantias principais da arquitetura

A arquitetura existe para garantir os seguintes comportamentos:

```text
uma operação financeira não movimenta dinheiro duas vezes

wallet nunca termina com saldo negativo

uma operação concluída pode ser reproduzida sem reaplicar saldo

ledger mantém histórico financeiro imutável

wallet + wager + ledger + outbox são confirmados atomicamente

mensagens SQS podem ser entregues novamente sem duplicar movimento

referências ausentes sobrevivem a restart

múltiplas instâncias podem processar wallets sem mutex global

eventos confirmados não são perdidos antes da publicação
```

---

# 19. Testes

## Unitários

Validam principalmente regras de domínio:

```text
Currency
Money
Wallet
WalletLedgerEntry
WagerTransaction
```

Sem buscar 100% de coverage.

O objetivo é provar invariantes financeiras e regras de negócio relevantes.

## Integração

Devem provar o comportamento real com:

```text
PostgreSQL
Keycloak
LocalStack
múltiplas instâncias
```

Cenários importantes:

```text
duplicação HTTP
redelivery SQS
concorrência na mesma wallet
wallets diferentes em paralelo
idempotência entre HTTP e SQS
outbox concorrente
restart durante pending reference
falha depois do commit e antes do delete da mensagem
reconciliação
provider isolation
```

---

# 20. Regra de complexidade

Antes de adicionar uma abstração, worker, interface, helper ou camada nova, a pergunta é:

> Isso atende um requisito explícito do desafio, protege uma invariante financeira ou torna uma falha recuperável?

Se a resposta for não, a abstração não deve ser adicionada ainda.
