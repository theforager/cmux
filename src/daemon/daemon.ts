import {loadConfig} from "../core/config.js";
import {listAutoStartIssues} from "../core/linear.js";
import {listSessions} from "../core/state.js";
import {startAgent} from "../core/agents.js";

export async function runDaemon(options: {cwd: string; once?: boolean}): Promise<void> {
  const config = loadConfig();
  const auto = config.linear?.autoStart;
  if (!auto?.enabled) {
    throw new Error("linear.autoStart.enabled is not true in cmux config");
  }
  const interval = Math.max(10, auto.intervalSeconds ?? 60) * 1000;
  const mode = auto.mode ?? "prepare";
  const maxConcurrent = auto.maxConcurrent ?? 3;

  async function tick(): Promise<void> {
    const known = new Set(listSessions().map(session => session.linear?.identifier ?? session.id));
    const running = listSessions().filter(session => session.status === "running").length;
    let available = Math.max(0, maxConcurrent - running);
    if (available === 0) return;
    const issues = await listAutoStartIssues();
    for (const issue of issues) {
      if (available === 0) break;
      if (known.has(issue.identifier)) continue;
      await startAgent({
        cwd: options.cwd,
        issueKey: issue.identifier,
        prepareOnly: mode === "prepare"
      });
      available -= 1;
    }
  }

  await tick();
  if (options.once) return;
  setInterval(() => {
    tick().catch(error => {
      console.error(`[cmux daemon] ${error.message}`);
    });
  }, interval);
}
