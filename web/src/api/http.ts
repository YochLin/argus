import { getMockData } from "./mock";

export async function getJSON<T>(url: string): Promise<T> {
  try {
    const res = await fetch(url);
    if (!res.ok) {
      throw new Error(`${url}: HTTP ${res.status}`);
    }
    return (await res.json()) as T;
  } catch (e) {
    console.warn(`[API] fetch ${url} failed, serving mock data`, e);
    return getMockData(url) as T;
  }
}

export async function postJSON<T>(url: string, body: unknown): Promise<T> {
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    const data: unknown = await res.json().catch(() => ({}));
    if (!res.ok) {
      const message = (data as { error?: string })?.error ?? `HTTP ${res.status}`;
      throw new ApiError(res.status, message);
    }
    return data as T;
  } catch (e) {
    if (e instanceof ApiError) throw e;
    console.warn(`[API] POST ${url} failed, mock success`, e);
    if (url === "/api/login") return { ok: true } as unknown as T;
    if (url === "/api/import" || url === "/api/wealth/import")
      return { rows: [], applied: 0, dryRun: (body as { dryRun?: boolean }).dryRun ?? true } as unknown as T;
    return { message: "Mock operation completed successfully." } as unknown as T;
  }
}

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

// Unlike every other read endpoint, this one is behind the write auth cookie,
// so it can't go through getJSON — that helper swallows failures into mock
// data, which would turn a 401 into a page of fake settings instead of the
// login prompt. Same ApiError contract as postJSON.
export async function getJSONAuthed<T>(url: string): Promise<T> {
  const res = await fetch(url);
  const data: unknown = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new ApiError(res.status, (data as { error?: string })?.error ?? `HTTP ${res.status}`);
  }
  return data as T;
}
