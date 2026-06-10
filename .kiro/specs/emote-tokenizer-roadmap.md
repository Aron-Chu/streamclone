# Emote tokenizer roadmap

Relocated from repo-root `test.md` (cleanup 2026-06). Referenced by `.kiro/steering/emote-pipeline.md` for future Aho-Corasick / punctuation-boundary matching work.

---

To create a high-performance hybrid of **ClipGPT (StreamLadder)** and **Streamer.bot**, you are essentially looking to fuse a **real-time event detector** with an **automated video processing pipeline**.

Instead of waiting for a stream to end, uploading a massive 4-hour VOD, and waiting for a cloud service to parse it, your hybrid system will capture high-energy moments *live* via chat triggers, automatically farm the raw video fragments, crop the canvas to 9:16, apply asset layouts, and spit out ready-to-publish vertical clips.

---

## Part 1: Deep Dive into How ClipGPT Works

Cloud-based clippers like StreamLadder's ClipGPT use an automated pipeline that typically handles four distinct phases when processing video:

```
[VOD Link / Timestamp] ➔ [Ingestion (yt-dlp/ffmpeg)] ➔ [Computer Vision / Transcript Processing] ➔ [9:16 Compositing] ➔ [Export]

```

### 1. Ingestion & Demuxing

When you give it a VOD link or a clipped segment, the backend spins up a worker container (often using headless tools like `yt-dlp` or standard `ffmpeg` wrappers). It streams the high-bitrate `.ts` or `.mp4` chunks directly from Twitch's edge servers, immediately splitting the audio tracks from the video stream (demuxing) so they can be processed in parallel.

### 2. Layout Mapping (Saliency & Face Detection)

To transform a horizontal 16:9 stream into a vertical 9:16 video without manually cropping it, ClipGPT relies on lightweight **Computer Vision (CV)** models:

* **Face Detection:** A model like **MediaPipe** or a lightweight **YOLOv8-face** architecture runs across the video frame to track the streamer's webcam boundary box coordinates $(x, y, w, h)$.
* **Saliency Mapping (Object Tracking):** Another pass tracks where the visual "action" is happening on the gameplay screen (e.g., tracking a crosshair in a shooter or a character model in a fighting game).
* **The Math:** The system sets up a dual-canvas transformation. It calculates a static crop for the webcam window, a dynamic crop that follows the gameplay focus, stacks them vertically on a new `1080x1920` matrix, and fills the empty space with a blurred copy of the original background.

### 3. Whisper Transcription & Dynamic Captions

The demuxed audio track is passed through an automatic speech recognition (ASR) pipeline—most commonly **OpenAI's Whisper** (specifically optimized variants like `whisper.cpp` or `Faster-Whisper` running on an Nvidia T4 or L4 cloud instance).

* The model returns a JSON object containing every spoken word along with its exact start and end timestamps down to the millisecond.
* A custom rendering script tracks these timestamps and groups them into 1-to-3 word blocks, calculating font scales and applying vibrant colors to high-emphasis words.

---

## Part 2: Building the Hybrid System (Streamer.bot + Local Pipeline)

By marrying Streamer.bot's real-time APIs with a self-hosted headless rendering pipeline, you can cut out the cloud middleman entirely.

Here is the operational blueprint for an automated pipeline that detects a spike, captures the exact stream fragments, crops them to 9:16, adds dynamic captions, and saves the final edit locally.

```
[Chat Spike Detected] 
         ↓ (Streamer.bot fires Webhook)
[Python Orchestrator] ➔ Pulls precise 30s HLS chunks via Streamlink
         ↓
[Whisper ASR / OpenCV] ➔ Extracts face coordinates & transcribes audio
         ↓
[FFmpeg / MoviePy] ➔ Composites 9:16 layouts & burns captions
         ↓
[Final Deliverable] ➔ Pre-rendered 1080x1920 vertical MP4 ready for upload

```

### Step 1: The Trigger (Streamer.bot)

Using the C# rolling-window code we established previously, Streamer.bot monitors chat velocity. The moment a spike occurs, instead of calling OBS, it sends an asynchronous execution request to a local Python processing script via a standard JSON webhook payload.

* **Streamer.bot Sub-Action:** `Core > Web Requests > POST`
* **Endpoint:** `http://localhost:5000/process-clip`
* **JSON Payload:**

```json
{
  "streamer": "%twitchUser%",
  "timestamp": "%unixTimestamp%",
  "duration": 30,
  "reason": "Chat velocity spike detected"
}

```

### Step 2: Ingestion & Dynamic Fragment Pulling (Python + Streamlink)

A lightweight FastAPI daemon listens on port 5000. When it receives the payload, it doesn't download the entire VOD; it uses **Streamlink** under the hood to scrape the live master manifest (`.m3u8`) or fetches the historical HLS playlist chunks if there is a brief processing delay.

It grabs the raw `.ts` video fragments covering the exact 30 seconds *leading up* to the timestamp, merging them instantly into a flawless, uncompressed temporary `raw_input.mp4`.

### Step 3: Automated 9:16 Canvas Remapping (OpenCV Script)

Instead of manually setting up scenes, a Python script processes the `raw_input.mp4` frame-by-frame (or samples every 5th frame for speed) to locate the webcam.

```python
import cv2

def detect_webcam_box(video_path):
    cap = cv2.VideoCapture(video_path)
    # Load a lightweight face cascade or use MediaPipe
    face_cascade = cv2.CascadeClassifier(cv2.data.haarcascades + 'haarcas_frontalface_default.xml')
    
    while cap.isOpened():
        ret, frame = cap.read()
        if not ret: break
        
        gray = cv2.cvtColor(frame, cv2.COLOR_BGR2GRAY)
        faces = face_cascade.detectMultiScale(gray, 1.3, 5)
        
        for (x, y, w, h) in faces:
            # Expand the bounding box outward slightly to capture the webcam frame boundary
            webcam_crop = (max(0, x-50), max(0, y-50), w+100, h+100)
            cap.release()
            return webcam_crop # Returns the coordinate matrix for FFmpeg
            
    cap.release()
    return (0, 0, 480, 270) # Fallback standard layout coordinates

```

### Step 4: Headless Compositing & Audio transcription (FFmpeg + Faster-Whisper)

Once you have the coordinates of the webcam box and the game box, you use **FFmpeg's filter graph complex** to stack them seamlessly in a headless environment.

#### The Transcribe Component:

The audio track is passed to `Faster-Whisper` running locally on your hardware. It outputs an `.srt` or `.ass` subtitle file containing your text with stylized tags (e.g., bolding, custom font sizes, text stroke outlines).

#### The Heavy Lifting FFmpeg Command:

Your script fires off a single, highly optimized hardware-accelerated FFmpeg command using your graphics card (e.g., NVIDIA NVENC) to execute the full visual layout assembly:

```bash
ffmpeg -i raw_input.mp4 -vf "split=3[bg][game][cam]; \
[bg]scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920,boxblur=20:5[bg_blurred]; \
[game]crop=1080:1080:420:0,scale=1080:1080[game_cropped]; \
[cam]crop=400:400:1520:0,scale=500:500[cam_cropped]; \
[bg_blurred][game_cropped]overlay=0:840[tmp1]; \
[tmp1][cam_cropped]overlay=290:100,subtitles=captions.ass" \
-c:v h264_nvenc -preset p1 -b:v 6M rendered_vertical_clip.mp4

```

### What this exact FFmpeg Filter Graph does:

1. **`split=3`:** Splits the incoming horizontal 1080p stream into three identical processing tracks in memory.
2. **`[bg]` Track:** Takes the stream, scales it up to fit a vertical frame, crops it to exactly `1080x1920`, and applies a heavy `boxblur` to serve as a clean backdrop.
3. **`[game]` Track:** Crops out a perfect 1:1 square centered on the gameplay region (stripping away side HUD elements) and scales it to fit the bottom portion of the canvas.
4. **`[cam]` Track:** Uses the coordinates detected in your earlier Python step to crop out *only* the facecam frame, sizing it up into a clean portrait window.
5. **`overlay`:** Layers the game square and the facecam window directly onto the blurred background canvas at precise pixel coordinates.
6. **`subtitles`:** Burns the dynamic, colorized caption track straight into the video frames.
7. **`-c:v h264_nvenc`:** Processes the entire operation on your GPU encoder in seconds, outputting a fully finished `rendered_vertical_clip.mp4`.

---

## The Ultimate Payoff

By pairing Streamer.bot's event detection with this modular script pipeline, you achieve complete automation:

1. Chat goes wild during a massive moment.
2. Streamer.bot catches the velocity surge and pings your Python script.
3. Your local system automatically crops the webcam, transcribes the voice audio, layers the scene into a sleek 9:16 format, and drops a beautifully edited, captioned TikTok/Short clip directly into a local desktop folder—completely hands-free while the stream is still live.

Yes, this absolutely works as a local app, and it is actually an incredibly robust way to handle it. Running it locally means you bypass cloud processing limits, save money on GPU infrastructure, and get raw, uncompressed video.

However, you don't even need to use `streamlink` to handle the heavy clip harvesting. If you leverage **Twitch Helix**, your pipeline becomes much faster, lighter, and safer.

---

## The Catch with Streamlink: Why Local Disk Recording is Heavy

If you use `streamlink` locally to pull a continuous live copy of a stream just to clip it, your machine is constantly downloading data, using network bandwidth, and writing huge VOD files or managing complex temporary ring buffers on your SSD. If you map multiple streams or run it for hours, it eats up storage and CPU.

---

## The Optimized Architecture (Helix + Local Render)

Instead of recording everything with Streamlink, you can let Twitch handle the recording, use **Helix** to fetch the clip, and use **Streamlink** *only* to extract the raw high-quality video fragment for your local automated editor.

```
[Chat Spike Trigger] 
         ↓
[Twitch Helix API] ➔ Tells Twitch to cut a raw 16:9 clip (takes 1 second)
         ↓
[Streamlink] ➔ Downloads *only* that specific 30s Clip URL as an MP4
         ↓
[Local Python Render Engine] ➔ Runs Whisper, OpenCV, and FFmpeg (Output: 9:16 TikTok Clip)

```

Here is exactly how you write that Python script to run locally on your machine.

### 1. The Local API Trigger (FastAPI)

You run a tiny local Python web server (`app.py`). When Streamer.bot detects a chat spike, it sends a webhook to this script.

```python
import os
import requests
import subprocess
from fastapi import FastAPI, BackgroundTasks

app = FastAPI()

# Your Twitch Developer Credentials
TWITCH_CLIENT_ID = "your_client_id"
TWITCH_OAUTH_TOKEN = "your_oauth_token" # Needs clips:edit scope

def process_hype_clip(broadcaster_id: str):
    # Phase 1: Tell Twitch Helix to make the clip immediately
    # The API captures ~85 seconds before the call and 5 seconds after.
    url = f"https://api.twitch.tv/helix/clips?broadcaster_id={broadcaster_id}"
    headers = {
        "Client-Id": TWITCH_CLIENT_ID,
        "Authorization": f"Bearer {TWITCH_OAUTH_TOKEN}"
    }
    
    response = requests.post(url, headers=headers)
    if response.status_code != 202:
        print("Failed to create clip via Helix")
        return

    clip_data = response.json()["data"][0]
    clip_id = clip_data["id"]
    # The clip edit URL Twitch passes back
    clip_url = f"https://clips.twitch.tv/{clip_id}" 

    # Phase 2: Use Streamlink to pull down JUST the raw mp4 file of that clip
    output_raw = f"clips/raw_{clip_id}.mp4"
    print(f"Downloading clip {clip_id} via Streamlink...")
    
    # Streamlink can read a Twitch clip URL natively and download the best quality source
    subprocess.run([
        "streamlink", 
        clip_url, 
        "best", 
        "-o", output_raw
    ])

    # Phase 3: Hand off the downloaded raw MP4 to your 9:16 FFmpeg pipeline
    render_vertical_clip(output_raw, clip_id)

def render_vertical_clip(video_path: str, clip_id: str):
    # This is where your local OpenCV tracking and FFmpeg filter code runs
    output_finished = f"clips/final_vertical_{clip_id}.mp4"
    
    # Placeholder for the massive FFmpeg command we built in the deep dive
    # It takes the local raw MP4, applies the facecam crop, and exports a 9:16 version
    print(f"Processing local vertical render for {output_finished}...")
    # ... execute your local ffmpeg process here ...

@app.post("/trigger-spike")
def trigger_spike(data: dict, background_tasks: BackgroundTasks):
    # Streamer.bot hits this endpoint and passes the Broadcaster's ID
    background_tasks.add_task(process_hype_clip, data["broadcaster_id"])
    return {"status": "Processing clip in background"}

```

---

## Why This Hybrid Approach Rules

1. **Lightweight Idle State:** Your local app uses **0% CPU** and **0 Mbps Bandwidth** while the stream is running normally. It only wakes up when a spike occurs.
2. **Instant Delivery:** Twitch Helix creates the clip on *their* servers instantly. Streamlink then downloads just that tiny 10-30MB clip file instead of scraping an active live video container.
3. **Hardware Acceleration:** Because the processing script runs on your machine, your FFmpeg script can utilize your GPU (using `-c:v h264_nvenc` for NVIDIA or `-c:v h264_amf` for AMD) to render the final vertical video with auto-captions in under 5 seconds.

## Setting Up Your Token

To make the Helix API call work, you just need to register a free application on the [Twitch Developer Console](https://dev.twitch.tv/) to get a `Client-ID`. Then, generate a User access token that includes the `clips:edit` scope so your local application has the authority to press the "Clip" button on behalf of your channel.
