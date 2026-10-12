// Keys come from users and the API ("constructor" is a valid ID): own-property lookups only.
export const own = <T,>(table: Record<string, T>, key: string): T | undefined => (Object.hasOwn(table, key) ? table[key] : undefined);
