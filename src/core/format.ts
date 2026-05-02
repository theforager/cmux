export function slugify(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 80) || "task";
}

export function nowIso(): string {
  return new Date().toISOString();
}

export function age(value: string | number): string {
  const created = typeof value === "number" ? value * 1000 : Date.parse(value);
  const diff = Math.max(0, Date.now() - created);
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h`;
  return `${Math.floor(hours / 24)}d`;
}

export function pad(value: string, width: number): string {
  if (value.length >= width) return value.slice(0, Math.max(0, width - 1)) + "…";
  return value + " ".repeat(width - value.length);
}
