import Emusks from "emusks";

const port = Number(process.env.PORT || 8098);
const token = String(process.env.X_AUTH_TOKEN || process.env.EMUSKS_X_AUTH_TOKEN || "").trim();

const client = new Emusks();

async function ensureLogin() {
  if (!token) {
    throw new Error("X_AUTH_TOKEN or EMUSKS_X_AUTH_TOKEN required");
  }
  await client.login(token);
}

const server = Bun.serve({
  port,
  async fetch(req) {
    const url = new URL(req.url);
    if (url.pathname === "/healthz") {
      return Response.json({ ok: Boolean(token) });
    }
    const match = url.pathname.match(/^\/users\/([^/]+)\/timeline$/);
    if (!match) {
      return new Response("not found", { status: 404 });
    }
    if (!token) {
      return Response.json({ error: "token not configured" }, { status: 503 });
    }
    try {
      await ensureLogin();
      const username = decodeURIComponent(match[1]);
      const user = await client.users.getByUsername(username);
      const timeline = await client.timelines.userTweets(user.id, { count: 40 });
      const sinceParam = url.searchParams.get("since");
      const sinceMs = sinceParam ? Date.parse(sinceParam) : 0;
      const items = (timeline?.tweets || timeline || [])
        .filter((tweet) => {
          const text = String(tweet?.text || tweet?.fullText || "");
          return /has been banned/i.test(text);
        })
        .filter((tweet) => {
          if (!sinceMs) return true;
          const created = Date.parse(tweet?.createdAt || tweet?.created_at || "");
          return !Number.isFinite(created) || created >= sinceMs;
        })
        .map((tweet) => ({
          id: String(tweet.id || tweet.id_str || ""),
          text: String(tweet.text || tweet.fullText || ""),
          url: tweet.id ? `https://x.com/i/status/${tweet.id}` : "",
          createdAt: tweet.createdAt || tweet.created_at || new Date().toISOString(),
        }))
        .filter((row) => row.id && row.text);
      return Response.json({ items });
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      return Response.json({ error: message }, { status: 502 });
    }
  },
});

console.log(`x-ingest listening on :${server.port}`);
