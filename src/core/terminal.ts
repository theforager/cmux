export function enterFullScreen(): void {
  process.stdout.write("\x1b[?1049h\x1b[2J\x1b[H\x1b[?25l");
}

export function leaveFullScreen(): void {
  process.stdout.write("\x1b[?25h\x1b[?1049l");
}
