package application

import (
	"context"
	"errors"
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
				if errors.Is(err, ErrAlreadyExists) {
					return ErrWalletAlreadyExists
				}

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

// Reconcile compares the stored wallet balance with the balance rebuilt from the ledger.
func (s *WalletService) Reconcile(ctx context.Context, walletID string) (*WalletReconciliationResult, error) {
	id, err := uuid.Parse(walletID)
	if err != nil || id == uuid.Nil {
		return nil, ErrWalletNotFound
	}

	snapshot, err := s.ledger.ReconciliationSnapshot(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrWalletNotFound
		}

		return nil, err
	}

	difference, err := snapshot.StoredBalance.Sub(snapshot.CalculatedBalance)
	if err != nil {
		return nil, err
	}

	return &WalletReconciliationResult{
		WalletID:          id,
		StoredBalance:     snapshot.StoredBalance,
		CalculatedBalance: snapshot.CalculatedBalance,
		Difference:        difference,
		Consistent:        difference.IsZero(),
		CheckedEntries:    snapshot.CheckedEntries,
	}, nil
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
	processed, err := processedEventData(opening)
	if err != nil {
		return err
	}

	if err := s.outbox.Create(ctx, NewWagerTransactionProcessedEvent(metadata, occurredAt, processed)); err != nil {
		return err
	}

	return s.outbox.Create(ctx, NewWalletBalanceChangedEvent(metadata, occurredAt, balanceChangedEventData(wallet, entry)))
}
