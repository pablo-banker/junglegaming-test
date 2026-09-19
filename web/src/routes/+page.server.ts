import { redirect } from '@sveltejs/kit';

/** The interface starts at the providers list. */
export function load() {
	redirect(307, '/providers');
}
