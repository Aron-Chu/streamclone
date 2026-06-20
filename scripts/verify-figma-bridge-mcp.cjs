#!/usr/bin/env node
/** MCP stdio smoke test for @gethopp/figma-mcp-bridge. */
const { spawn } = require("child_process");

const PACKAGE = "@gethopp/figma-mcp-bridge@0.0.15";
const isWin = process.platform === "win32";
const child = isWin
  ? spawn("cmd.exe", ["/c", "npx", "-y", PACKAGE], { stdio: ["pipe", "pipe", "pipe"] })
  : spawn("npx", ["-y", PACKAGE], { stdio: ["pipe", "pipe", "pipe"] });

let nextId = 1;
let stdout = "";
let completed = false;
const pending = new Map();
let stderr = "";
let markReady;
const ready = new Promise((resolve) => {
  markReady = resolve;
});
const overallTimer = setTimeout(() => {
  if (completed) return;
  console.error("FAIL: overall timeout");
  child.kill();
  process.exit(1);
}, 35000);

function writeMessage(message) {
  child.stdin.write(`${JSON.stringify(message)}\n`);
}

function send(method, params = {}) {
  const id = nextId++;
  writeMessage({ jsonrpc: "2.0", id, method, params });
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      pending.delete(id);
      reject(new Error(`timeout waiting for ${method}`));
    }, 25000);
    pending.set(id, { resolve, reject, timer });
  });
}

function handleLine(line) {
  if (!line.trim()) return;
  let message;
  try {
    message = JSON.parse(line);
  } catch {
    return;
  }
  if (message.id && pending.has(message.id)) {
    const request = pending.get(message.id);
    pending.delete(message.id);
    clearTimeout(request.timer);
    if (message.error) request.reject(new Error(JSON.stringify(message.error)));
    else request.resolve(message.result);
  }
}

child.stdout.on("data", (chunk) => {
  stdout += chunk.toString("utf8");
  const lines = stdout.split(/\r?\n/);
  stdout = lines.pop() ?? "";
  for (const line of lines) handleLine(line);
});
child.stderr.on("data", (chunk) => {
  const text = chunk.toString("utf8");
  process.stderr.write(chunk);
  stderr += text;
  if (stderr.includes("Starting MCP server")) markReady();
});
child.on("exit", (code) => {
  if (completed) return;
  const error = new Error(`bridge exited before verification completed: ${code ?? "unknown"}`);
  for (const request of pending.values()) {
    clearTimeout(request.timer);
    request.reject(error);
  }
  pending.clear();
});

(async () => {
  await Promise.race([
    ready,
    new Promise((_, reject) => setTimeout(() => reject(new Error("bridge server did not become ready")), 20000)),
  ]);
  await send("initialize", {
    protocolVersion: "2024-11-05",
    capabilities: {},
    clientInfo: { name: "streamclone-verify", version: "1.0" },
  });
  writeMessage({ jsonrpc: "2.0", method: "notifications/initialized" });
  const result = await send("tools/list", {});
  const names = (result.tools ?? []).map((tool) => tool.name);
  process.stderr.write(`figma-bridge tools: ${names.length}\n`);
  names.slice(0, 12).forEach((name) => process.stderr.write(`  - ${name}\n`));
  if (!names.includes("list_files")) throw new Error("list_files missing");
  if (!names.includes("get_node")) throw new Error("get_node missing");
  if (!names.includes("get_screenshot")) throw new Error("get_screenshot missing");
  completed = true;
  clearTimeout(overallTimer);
  process.stderr.write("ok figma-bridge MCP stdio handshake\n");
  child.kill();
  setTimeout(() => process.exit(0), 25);
})().catch((error) => {
  clearTimeout(overallTimer);
  console.error(`FAIL: ${error.message}`);
  child.kill();
  process.exit(1);
});
