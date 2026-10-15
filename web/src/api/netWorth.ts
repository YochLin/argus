// The sidebar's net-worth card (App) lives apart from the pages that write the
// numbers it shows, so every write that moves net worth tells it to reload —
// here, once, rather than from each page that adds, edits, archives or imports.
const netWorthListeners = new Set<() => void>();

export function onNetWorthChange(listener: () => void): () => void {
  netWorthListeners.add(listener);
  return () => void netWorthListeners.delete(listener);
}

export async function movesNetWorth<T>(write: Promise<T>): Promise<T> {
  const result = await write;
  netWorthListeners.forEach((notify) => notify());
  return result;
}
