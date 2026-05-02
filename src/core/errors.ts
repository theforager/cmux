export class CmuxError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "CmuxError";
  }
}

export function assert(condition: unknown, message: string): asserts condition {
  if (!condition) {
    throw new CmuxError(message);
  }
}
