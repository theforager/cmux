import {existsSync, mkdirSync, readFileSync} from "node:fs";
import {dirname, join, resolve} from "node:path";
import {homedir} from "node:os";
import type {CmuxConfig} from "./types.js";

function configHome(): string {
  return process.env.XDG_CONFIG_HOME ?? join(homedir(), ".config");
}

function stateHome(): string {
  return process.env.XDG_STATE_HOME ?? join(homedir(), ".local", "state");
}

export function configPath(): string {
  return join(configHome(), "cmux", "config.json");
}

export function loadConfig(): CmuxConfig {
  const path = configPath();
  if (!existsSync(path)) return {};
  return JSON.parse(readFileSync(path, "utf8")) as CmuxConfig;
}

export function getStateDir(config = loadConfig()): string {
  const configured = process.env.CMUX_STATE_DIR ?? config.stateDir;
  return resolve(configured ?? join(stateHome(), "cmux"));
}

export function getAgentsDir(config = loadConfig()): string {
  return join(getStateDir(config), "agents");
}

export function getHooksDir(): string {
  return join(configHome(), "cmux", "hooks");
}

export function ensureStateDirs(config = loadConfig()): void {
  mkdirSync(getAgentsDir(config), {recursive: true});
}

export function defaultWorktreeRoot(repoPath: string, config = loadConfig()): string {
  if (config.worktreeRoot) return resolve(repoPath, config.worktreeRoot);
  return join(dirname(repoPath), "worktrees");
}
