#!/usr/bin/env node
/** Call figma-bridge list_files + optional metadata/node probes via MCP stdio. */
const { spawn } = require("child_process");

const PACKAGE = "@gethopp/figma-mcp-bridge@0.0.15";
const DEFAULT_FILE_KEY = process.env.FIGMA_FILE_KEY || "YDMQSWrJHyA7g5D5pIfruH";
const DEFAULT_NODE_ID = process.env.FIGMA_NODE_ID || "8:2";

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
}, 60000);

function writeMessage(message) {
  child.stdin.write(`${JSON.stringify(message)}\n`);
}

function send(method, params = {}) {
  const id = nextId++;
  writeMessage({ jsonrpc: "2.0", id, method, params });
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      pending.delete(id);
      reject(new Error(`timeout: ${method}`));
    }, 20000);
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

function contentText(result) {
  const content = result?.content;
  if (!Array.isArray(content)) return "";
  return content.map((part) => part.text || "").join("");
}

function printResult(label, result, max = 1600) {
  const text = contentText(result) || JSON.stringify(result, null, 2);
  process.stderr.write(`${label}:\n`);
  process.stderr.write(`${text.slice(0, max)}${text.length > max ? "\n..." : ""}\n`);
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
  const error = new Error(`bridge exited before live probe completed: ${code ?? "unknown"}`);
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
    clientInfo: { name: "streamclone-live-test", version: "1.0" },
  });
  writeMessage({ jsonrpc: "2.0", method: "notifications/initialized" });

  const files = await send("tools/call", { name: "list_files", arguments: {} });
  printResult("list_files", files);

  const filesText = contentText(files);
  if (!filesText.includes(DEFAULT_FILE_KEY)) {
    throw new Error(`Figma file ${DEFAULT_FILE_KEY} is not connected. Run the Figma MCP Bridge plugin in that file.`);
  }

  const metadata = await send("tools/call", {
    name: "get_metadata",
    arguments: { fileKey: DEFAULT_FILE_KEY },
  });
  printResult("get_metadata", metadata);

  const node = await send("tools/call", {
    name: "get_node",
    arguments: { fileKey: DEFAULT_FILE_KEY, nodeId: DEFAULT_NODE_ID },
  });
  printResult(`get_node ${DEFAULT_NODE_ID}`, node);

  completed = true;
  clearTimeout(overallTimer);
  process.stderr.write("ok figma-bridge live file probe\n");
  child.kill();
  setTimeout(() => process.exit(0), 25);
})().catch((error) => {
  clearTimeout(overallTimer);
  console.error(`FAIL: ${error.message}`);
  child.kill();
  process.exit(1);
});
