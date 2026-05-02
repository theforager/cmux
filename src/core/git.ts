import {existsSync, mkdirSync} from "node:fs";
import {basename, join} from "node:path";
import {defaultWorktreeRoot, loadConfig} from "./config.js";
import {slugify} from "./format.js";
import {run} from "./process.js";

export async function gitRoot(cwd: string): Promise<string> {
  const {stdout} = await run("git", ["rev-parse", "--show-toplevel"], {cwd});
  return stdout.trim();
}

export async function currentBranch(cwd: string): Promise<string> {
  const {stdout} = await run("git", ["branch", "--show-current"], {cwd});
  return stdout.trim();
}

async function branchExists(repoPath: string, branch: string): Promise<boolean> {
  try {
    await run("git", ["show-ref", "--verify", "--quiet", `refs/heads/${branch}`], {cwd: repoPath});
    return true;
  } catch {
    return false;
  }
}

export async function ensureWorktree(options: {
  cwd: string;
  identifier: string;
  title: string;
  branchName?: string;
}): Promise<{repoPath: string; branch: string; worktreePath: string}> {
  const repoPath = await gitRoot(options.cwd);
  const branch = options.branchName && options.branchName.trim()
    ? options.branchName.trim()
    : `agent/${options.identifier}-${slugify(options.title)}`;
  const root = defaultWorktreeRoot(repoPath, loadConfig());
  mkdirSync(root, {recursive: true});
  const worktreePath = join(root, `${options.identifier}-${slugify(options.title)}`);

  if (!existsSync(worktreePath)) {
    if (await branchExists(repoPath, branch)) {
      await run("git", ["worktree", "add", worktreePath, branch], {cwd: repoPath});
    } else {
      await run("git", ["worktree", "add", "-b", branch, worktreePath], {cwd: repoPath});
    }
  }

  return {repoPath, branch, worktreePath};
}

export function fallbackRepoPath(cwd: string): string {
  return cwd.endsWith(`/${basename(cwd)}`) ? cwd : cwd;
}
