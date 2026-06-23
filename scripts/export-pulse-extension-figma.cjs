#!/usr/bin/env node
/** Export Pulse extension Figma frames via figma-bridge save_screenshots. */
const { spawn } = require("child_process");
const path = require("path");

const PACKAGE = "@gethopp/figma-mcp-bridge@0.0.15";
const repoRoot = path.resolve(__dirname, "..");
const outDir = path.join(repoRoot, "docs", "pulse-extension", "figma");

const FRAMES = [
  { nodeId: "2:2", file: "board-full.png" },
  { nodeId: "2:10", file: "hero-on-twitch.png" },
  { nodeId: "1:4", file: "expanded-panel.png" },
  { nodeId: "2:121", file: "toolbar-popup.png" },
  { nodeId: "2:122", file: "settings.png" },
  { nodeId: "2:123", file: "top-emotes.png" },
  { nodeId: "2:124", file: "live-seek-states.png" },
  { nodeId: "2:125", file: "state-mini-dock.png" },
  { nodeId: "2:126", file: "state-collapsed.png" },
  { nodeId: "2:127", file: "state-warming-up.png" },
  { nodeId: "2:128", file: "state-cannot-reach-backend.png" },
  { nodeId: "2:249", file: "saved-moments.png" },
  { nodeId: "2:250", file: "per-signal-lanes.png" },
  { nodeId: "2:251", file: "stream-recap.png" },
];

const isWin = process.platform === "win32";
const child = isWin
  ? spawn("cmd.exe", ["/c", "npx", "-y", PACKAGE], { stdio: ["pipe", "pipe", "pipe"] })
  : spawn("npx", ["-y", PACKAGE], { stdio: ["pipe", "pipe", "pipe"] });

let nextId = 1;
let stdout = "";
let completed = false;
const pending = new Map();
let markReady;
const ready = new Promise((resolve) => {
  markReady = resolve;
});

const overallTimer = setTimeout(() => {
  if (completed) return;
  console.error("FAIL: overall timeout");
  child.kill();
  process.exit(1);
}, 120000);

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
    }, 90000);
    pending.set(id, { resolve, reject, timer });
  });
}

function callTool(name, args = {}) {
  return send("tools/call", { name, arguments: args });
}

child.stdout.on("data", (chunk) => {
  stdout += chunk.toString();
  const lines = stdout.split("\n");
  stdout = lines.pop() || "";
  for (const line of lines) {
    if (!line.trim()) continue;
    try {
      const msg = JSON.parse(line);
      if (msg.id && pending.has(msg.id)) {
        const { resolve, reject, timer } = pending.get(msg.id);
        clearTimeout(timer);
        pending.delete(msg.id);
        if (msg.error) reject(new Error(JSON.stringify(msg.error)));
        else resolve(msg.result);
      }
      if (msg.method === "notifications/initialized") markReady();
    } catch {
      /* ignore non-json */
    }
  }
});

child.stderr.on("data", (d) => process.stderr.write(d));

async function main() {
  writeMessage({ jsonrpc: "2.0", method: "initialize", params: {
    protocolVersion: "2024-11-05",
    capabilities: {},
    clientInfo: { name: "export-pulse-extension-figma", version: "1.0.0" },
  }, id: nextId++ });
  await ready;

  const list = await callTool("list_files", {});
  const text = list?.content?.[0]?.text || JSON.stringify(list);
  let files;
  try {
    files = JSON.parse(text);
  } catch {
    throw new Error(`list_files failed: ${text}`);
  }
  if (!Array.isArray(files) || files.length === 0) {
    throw new Error(
      "No Figma file connected. Open Streamclone Pulse — Chrome Extension in Figma desktop and run Figma MCP Bridge plugin."
    );
  }
  const fileKey = files[0].fileKey;
  process.stderr.write(`connected: ${files[0].fileName} (${fileKey})\n`);

  const items = FRAMES.map(({ nodeId, file }) => ({
    nodeId,
    outputPath: path.join("docs", "pulse-extension", "figma", file).replace(/\\/g, "/"),
  }));

  const result = await callTool("save_screenshots", {
    fileKey,
    format: "PNG",
    scale: 2,
    items,
  });

  const body = result?.content?.[0]?.text || JSON.stringify(result);
  process.stderr.write(body + "\n");
  const parsed = JSON.parse(body);
  if (parsed.failed > 0) {
    throw new Error(`export failed: ${parsed.failed}/${parsed.total}`);
  }
  process.stderr.write(`ok exported ${parsed.succeeded} PNGs to ${outDir}\n`);
}

main()
  .then(() => {
    completed = true;
    clearTimeout(overallTimer);
    child.kill();
    process.exit(0);
  })
  .catch((err) => {
    completed = true;
    clearTimeout(overallTimer);
    console.error("FAIL:", err.message);
    child.kill();
    process.exit(1);
  });
