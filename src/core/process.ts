import {spawn} from "node:child_process";
import {statSync} from "node:fs";

export interface RunResult {
  stdout: string;
  stderr: string;
}

export function run(command: string, args: string[], options: {cwd?: string; input?: string} = {}): Promise<RunResult> {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd: options.cwd,
      stdio: ["pipe", "pipe", "pipe"]
    });
    let stdout = "";
    let stderr = "";
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", chunk => {
      stdout += chunk;
    });
    child.stderr.on("data", chunk => {
      stderr += chunk;
    });
    child.on("error", reject);
    child.on("close", code => {
      if (code === 0) {
        resolve({stdout, stderr});
      } else {
        const error = new Error(`${command} ${args.join(" ")} failed with exit ${code}\n${stderr.trim()}`);
        reject(error);
      }
    });
    if (options.input !== undefined) {
      child.stdin.end(options.input);
    } else {
      child.stdin.end();
    }
  });
}

export async function commandExists(command: string): Promise<boolean> {
  try {
    await run("sh", ["-lc", `command -v "$1" >/dev/null 2>&1`, "sh", command]);
    return true;
  } catch {
    return false;
  }
}

export function shellQuote(value: string): string {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

export function isExecutable(path: string): boolean {
  try {
    const stat = statSync(path);
    return stat.isFile() && Boolean(stat.mode & 0o111);
  } catch {
    return false;
  }
}
