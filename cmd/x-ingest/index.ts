import Emusks from "emusks";

const PORT = Number(process.env.PORT || "8098");
const authToken =
  process.env.X_AUTH_TOKEN?.trim() || process.env.EMUSKS_X_AUTH_TOKEN?.trim() || "";

let client: Emusks | null = null;
let loginError: string | null = null;

async function ensureClient(): Promise<Emusks> {
  if (!authToken) {
    throw new Error("X_AUTH_TOKEN or EMUSKS_X_AUTH_TOKEN is required");
  }
  if (client) {
    return client;
  }
  if (loginError) {
    throw new Error(loginError);
  }
  const next = new Emusks();
  try {
    await next.login(authToken);
    client = next;
    return client;
  } catch (err) {
    loginError = err instanceof Error ? err.message : String(err);
    throw err;
  }
}

function tweetURL(id: string): string {
  return `https://x.com/i/status/${id}`;
}

function parseSince(raw: string): Date | null {
  const value = raw.trim();
  if (!value) {
    return null;
  }
  const ts = Date.parse(value);
  if (Number.isNaN(ts)) {
    return null;
  }
  return new Date(ts);
}

async function streamerBansTimeline(since: Date | null) {
  const emusks = await ensureClient();
  const user = await emusks.users.getByUsername("StreamerBans");
  const timeline = await emusks.timelines.userTweets(user.id, { count: 40 });
  const tweets = Array.isArray(timeline?.tweets) ? timeline.tweets : timeline ?? [];
  const items = [];
  for (const tweet of tweets) {
    const id = String(tweet?.id ?? "");
    const text = String(tweet?.text ?? tweet?.fullText ?? "").trim();
    if (!id || !text) {
      continue;
    }
    const createdAt = tweet?.createdAt ? new Date(tweet.createdAt) : null;
    if (since && createdAt && createdAt < since) {
      continue;
    }
    items.push({
      id,
      text,
      url: tweetURL(id),
      createdAt: createdAt ? createdAt.toISOString() : "",
    });
    if (items.length >= 40) {
      break;
    }
  }
  return items;
}

Bun.serve({
  port: PORT,
  async fetch(req) {
    const url = new URL(req.url);
    if (req.method !== "GET") {
      return new Response("method not allowed", { status: 405 });
    }
    if (url.pathname === "/healthz") {
      if (!authToken) {
        return new Response("no auth token", { status: 503 });
      }
      try {
        await ensureClient();
        return new Response("ok");
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        return new Response(message, { status: 503 });
      }
    }
    if (url.pathname === "/users/StreamerBans/timeline") {
      if (!authToken) {
        return Response.json({ error: "no auth token" }, { status: 503 });
      }
      try {
        const since = parseSince(url.searchParams.get("since") ?? "");
        const items = await streamerBansTimeline(since);
        return Response.json({ items });
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        return Response.json({ error: message }, { status: 502 });
      }
    }
    return new Response("not found", { status: 404 });
  },
});

console.log(`x-ingest listening on :${PORT}`);
