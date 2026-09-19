package application

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

type CreateWalletResult struct {
	WalletID uuid.UUID    `json:"walletId"`
	PlayerID uuid.UUID    `json:"playerId"`
	Balance  domain.Money `json:"balance"`
	Version  int64        `json:"version"`
}

type WalletResult struct {
	WalletID uuid.UUID    `json:"walletId"`
	PlayerID uuid.UUID    `json:"playerId"`
	Balance  domain.Money `json:"balance"`
	Version  int64        `json:"version"`
}

type WalletLedgerResult struct {
	Entries []*domain.WalletLedgerEntry
}

type WalletReconciliationResult struct {
	WalletID          uuid.UUID
	StoredBalance     domain.Money
	CalculatedBalance domain.Money
	Difference        domain.Money
	Consistent        bool
	CheckedEntries    int64
}

type WalletService struct {
	txManager TransactionManager
	clock     Clock
	wallets   WalletRepository
	wagers    WagerRepository
	ledger    WalletLedgerRepository
	outbox    OutboxRepository
}

// NewWalletService creates the wallet application service.
func NewWalletService(
	txManager TransactionManager,
	clock Clock,
	wallets WalletRepository,
	wagers WagerRepository,
	ledger WalletLedgerRepository,
	outbox OutboxRepository,
) *WalletService {
	return &WalletService{
		txManager: txManager,
		clock:     clock,
		wallets:   wallets,
		wagers:    wagers,
		ledger:    ledger,
		outbox:    outbox,
	}
}

// Create creates a wallet and its opening financial records when required.
func (s *WalletService) Create(ctx context.Context, command CreateWalletCommand, metadata CommandMetadata) (*CreateWalletResult, error) {
	if strings.TrimSpace(metadata.CorrelationID) == "" {
		return nil, ErrInvalidCorrelationID
	}

	playerID, err := uuid.Parse(command.PlayerID)
	if err != nil || playerID == uuid.Nil {
		return nil, domain.ErrInvalidPlayerID
	}

	currency, err := domain.NewCurrency(command.Currency)
	if err != nil {
		return nil, err
	}

	initialBalance, err := domain.ParseMoney(
		command.InitialBalance,
		currency,
	)
	if err != nil {
		return nil, err
	}

	var result *CreateWalletResult

	err = s.txManager.WithinTransaction(
		ctx,
		func(txCtx context.Context) error {
			now, err := s.clock.Now(txCtx)
			if err != nil {
				return err
			}

			wallet, err := domain.NewWallet(
				uuid.New(),
				playerID,
				initialBalance,
				now,
			)
			if err != nil {
				return err
			}

			if err := s.wallets.Create(txCtx, wallet); err != nil {
				return err
			}

			if !initialBalance.IsZero() {
				if err := s.createOpening(
					txCtx,
					wallet,
					initialBalance,
					metadata,
					now,
				); err != nil {
					return err
				}
			}

			result = &CreateWalletResult{
				WalletID: wallet.ID(),
				PlayerID: wallet.PlayerID(),
				Balance:  wallet.Balance(),
				Version:  wallet.Version(),
			}

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Get returns a wallet by its identifier.
func (s *WalletService) Get(ctx context.Context, walletID string) (*WalletResult, error) {
	id, err := uuid.Parse(walletID)
	if err != nil || id == uuid.Nil {
		return nil, ErrWalletNotFound
	}

	wallet, err := s.wallets.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return &WalletResult{
		WalletID: wallet.ID(),
		PlayerID: wallet.PlayerID(),
		Balance:  wallet.Balance(),
		Version:  wallet.Version(),
	}, nil
}

// ListLedger returns paginated ledger entries for a wallet.
func (s *WalletService) ListLedger(
	ctx context.Context,
	walletID string,
	beforeCreatedAt *time.Time,
	beforeID *uuid.UUID,
	limit int,
) (*WalletLedgerResult, error) {
	id, err := uuid.Parse(walletID)
	if err != nil || id == uuid.Nil {
		return nil, ErrWalletNotFound
	}

	if _, err := s.wallets.FindByID(ctx, id); err != nil {
		return nil, err
	}

	entries, err := s.ledger.ListByWallet(
		ctx,
		id,
		beforeCreatedAt,
		beforeID,
		limit,
	)
	if err != nil {
		return nil, err
	}

	return &WalletLedgerResult{
		Entries: entries,
	}, nil
}

// Reconcile compares the stored wallet balance with the reconstructed ledger balance.
func (s *WalletService) Reconcile(ctx context.Context, walletID string) (*WalletReconciliationResult, error) {
	id, err := uuid.Parse(walletID)
	if err != nil || id == uuid.Nil {
		return nil, ErrWalletNotFound
	}

	var result *WalletReconciliationResult

	err = s.txManager.WithinTransaction(
		ctx,
		func(txCtx context.Context) error {
			wallet, err := s.wallets.FindByIDForUpdate(txCtx, id)
			if err != nil {
				return err
			}

			calculatedBalance, checkedEntries, err := s.ledger.CalculateBalance(txCtx, wallet.ID(), wallet.Balance().Currency())
			if err != nil {
				return err
			}

			difference, err := wallet.Balance().Sub(calculatedBalance)
			if err != nil {
				return err
			}

			result = &WalletReconciliationResult{
				WalletID:          wallet.ID(),
				StoredBalance:     wallet.Balance(),
				CalculatedBalance: calculatedBalance,
				Difference:        difference,
				Consistent:        wallet.Balance().Equal(calculatedBalance),
				CheckedEntries:    checkedEntries,
			}

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// createOpening creates the financial records for a positive initial balance.
func (s *WalletService) createOpening(
	ctx context.Context,
	wallet *domain.Wallet,
	initialBalance domain.Money,
	metadata CommandMetadata,
	occurredAt time.Time,
) error {
	opening, err := domain.NewOpeningWagerTransaction(domain.NewOpeningWagerTransactionParams{
		ID:        uuid.New(),
		WalletID:  wallet.ID(),
		PlayerID:  wallet.PlayerID(),
		Amount:    initialBalance,
		CreatedAt: occurredAt,
	})
	if err != nil {
		return err
	}

	if err := s.wagers.Create(ctx, opening); err != nil {
		return err
	}

	zero, err := domain.Zero(initialBalance.Currency())
	if err != nil {
		return err
	}

	entry, err := domain.NewWalletLedgerEntry(
		uuid.New(),
		wallet.ID(),
		opening.ID(),
		domain.WalletLedgerDirectionCredit,
		initialBalance,
		zero,
		initialBalance,
		occurredAt,
	)
	if err != nil {
		return err
	}

	if err := s.ledger.Create(ctx, entry); err != nil {
		return err
	}

	if err := s.createOpeningEvents(
		ctx,
		wallet,
		opening,
		entry,
		metadata,
		occurredAt,
	); err != nil {
		return err
	}

	return nil
}

// createOpeningEvents persists the events produced by a positive wallet opening.
func (s *WalletService) createOpeningEvents(
	ctx context.Context,
	wallet *domain.Wallet,
	opening *domain.WagerTransaction,
	entry *domain.WalletLedgerEntry,
	metadata CommandMetadata,
	occurredAt time.Time,
) error {
	processedEvent := EventEnvelope{
		EventID:       uuid.New(),
		EventType:     EventTypeWagerTransactionProcessed,
		AggregateID:   opening.ID(),
		CorrelationID: metadata.CorrelationID,
		CausationID:   metadata.CausationID,
		Version:       EventVersion,
		OccurredAt:    occurredAt,
		Payload: WagerTransactionProcessedPayload{
			TransactionID: opening.ID(),
			WalletID:      wallet.ID(),
			PlayerID:      wallet.PlayerID(),
			Type:          opening.Type(),
			Amount:        opening.Amount(),
			BalanceBefore: entry.BalanceBefore(),
			BalanceAfter:  entry.BalanceAfter(),
		},
	}

	if err := s.outbox.Create(ctx, processedEvent); err != nil {
		return err
	}

	balanceChangedEvent := EventEnvelope{
		EventID:       uuid.New(),
		EventType:     EventTypeWalletBalanceChanged,
		AggregateID:   wallet.ID(),
		CorrelationID: metadata.CorrelationID,
		CausationID:   metadata.CausationID,
		Version:       EventVersion,
		OccurredAt:    occurredAt,
		Payload: WalletBalanceChangedPayload{
			WalletID:      wallet.ID(),
			PlayerID:      wallet.PlayerID(),
			TransactionID: opening.ID(),
			Direction:     entry.Direction(),
			Amount:        entry.Amount(),
			BalanceBefore: entry.BalanceBefore(),
			BalanceAfter:  entry.BalanceAfter(),
			WalletVersion: wallet.Version(),
		},
	}

	if err := s.outbox.Create(ctx, balanceChangedEvent); err != nil {
		return err
	}

	return nil
}
