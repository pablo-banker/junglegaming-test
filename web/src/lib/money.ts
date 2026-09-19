// Money helpers. Amounts are decimal strings with two places; arithmetic uses integer cents.

/** The only amount format the API accepts. */
export const AMOUNT_PATTERN = /^(0|[1-9][0-9]{0,17})\.[0-9]{2}$/;

/** Formats integer cents as a decimal string, e.g. 2550 -> "25.50". */
export function amountFromCents(cents: number): string {
	if (!Number.isSafeInteger(cents) || cents < 0) {
		throw new Error(`invalid cents: ${cents}`);
	}

	return `${Math.trunc(cents / 100)}.${String(cents % 100).padStart(2, '0')}`;
}

/** Returns a random amount between two integer cent values, inclusive. */
export function randomAmount(minCents: number, maxCents: number): string {
	return amountFromCents(minCents + Math.floor(Math.random() * (maxCents - minCents + 1)));
}

/** Formats a decimal string with thousands separators, without converting it to a number. */
export function formatAmount(amount: string | null | undefined): string {
	if (amount === null || amount === undefined || amount === '') return '—';

	const negative = amount.startsWith('-');
	const [integer, fraction = '00'] = amount.replace('-', '').split('.');
	const grouped = integer.replace(/\B(?=(\d{3})+(?!\d))/g, '.');

	return `${negative ? '-' : ''}${grouped},${fraction}`;
}

/** Formats money for display, e.g. "BRL 1.000,00". */
export function formatMoney(amount: string | null | undefined, currency = 'BRL'): string {
	return amount === null || amount === undefined ? '—' : `${currency} ${formatAmount(amount)}`;
}
