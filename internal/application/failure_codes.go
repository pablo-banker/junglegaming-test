package application

// Stable failure codes of the external contract; they must never be renamed.
const (
	// FailureCodeBetInsufficientFunds rejects a BET larger than the wallet balance.
	FailureCodeBetInsufficientFunds = "BET_INSUFFICIENT_FUNDS"

	// FailureCodeReversalInsufficientFunds rejects a ROLLBACK that would need to debit more than the balance.
	FailureCodeReversalInsufficientFunds = "REVERSAL_INSUFFICIENT_FUNDS"

	// FailureCodeReferenceNotFound rejects a reference that did not arrive before the retry TTL.
	FailureCodeReferenceNotFound = "REFERENCE_NOT_FOUND"

	// FailureCodeReferenceNotProcessable rejects a reference that ended REJECTED or FAILED.
	FailureCodeReferenceNotProcessable = "REFERENCE_NOT_PROCESSABLE"

	// FailureCodeReferenceMismatch rejects a reference whose provider, player, wallet, currency, round or amount differ.
	FailureCodeReferenceMismatch = "REFERENCE_MISMATCH"

	// FailureCodeReferenceTypeNotAllowed rejects a reference of a type the operation cannot reverse or follow.
	FailureCodeReferenceTypeNotAllowed = "REFERENCE_TYPE_NOT_ALLOWED"

	// FailureCodeDuplicateReversal rejects a second successful REFUND or ROLLBACK of the same transaction.
	FailureCodeDuplicateReversal = "DUPLICATE_REVERSAL"

	// FailureCodeProcessingFailed marks a FAILED transaction after repeated unexpected processing errors.
	FailureCodeProcessingFailed = "PROCESSING_FAILED"
)
