"""Render offline model frames as a GIF and a shareable MP4. Requires Pillow/FFmpeg."""
import json
import os
from pathlib import Path
import subprocess
import tempfile

from PIL import Image, ImageDraw, ImageFont

ROOT = Path(__file__).resolve().parents[2]
os.chdir(ROOT)
font_path = os.environ.get("DEMO_FONT")
if not font_path:
    font_path = next((p for p in (
        "/System/Library/Fonts/Menlo.ttc",
        "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
    ) if Path(p).exists()), None)
if not font_path:
    raise SystemExit("Set DEMO_FONT to a monospace TTF/TTC font.")
font = ImageFont.truetype(font_path, 18)
label = ImageFont.truetype(font_path, 20)
frames = json.loads(subprocess.check_output(["go", "run", "./scripts/demo-animation"], text=True))
assets = ROOT / "docs/assets"
images = []
with tempfile.TemporaryDirectory(prefix="azpipe-demo-") as temp:
    entries = []
    for index, frame in enumerate(frames):
        im = Image.new("RGB", (1280, 900), "#101416")
        draw = ImageDraw.Draw(im)
        draw.rounded_rectangle((20, 18, 1260, 68), 10, fill="#202a30")
        draw.text((36, 31), frame["caption"], font=label, fill="#d3f536")
        lines = frame["view"].splitlines()
        if len(lines) > 33 or any(len(line) > 104 for line in lines):
            raise SystemExit(f"Frame {index} exceeds the recording canvas")
        for row, line in enumerate(lines):
            color = "#e8e9e8"
            if any(c in line for c in "╭╰│"):
                color = "#53cef0"
            if "█" in line or "AZPIPE" in line:
                color = "#d3f536"
            if "[x]" in line or ">" in line:
                draw.rectangle((30, 84 + row * 23, 1248, 107 + row * 23), fill="#164d63")
            # Fixed cell placement keeps box borders and table columns aligned.
            for col, char in enumerate(line):
                draw.text((32 + col * 12, 84 + row * 23), char, font=font, fill=color)
        draw.line((32, 857, 1248, 857), fill="#33434c", width=1)
        draw.text((32, 866), "AZPIPE  /  DEMO OFFLINE  /  DADOS FICTICIOS", font=font, fill="#99aab4")
        path = Path(temp) / f"frame-{index:02}.png"
        im.save(path)
        images.append(im)
        entries.extend([f"file '{path}'", f"duration {frame['seconds']}"])
    entries.append(f"file '{path}'")
    manifest = Path(temp) / "frames.txt"
    manifest.write_text("\n".join(entries) + "\n")
    subprocess.run([
        "ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "concat",
        "-safe", "0", "-i", str(manifest), "-vf", "fps=10", "-c:v", "libx264",
        "-crf", "20", "-pix_fmt", "yuv420p", "-movflags", "+faststart",
        "-t", str(sum(frame["seconds"] for frame in frames)),
        str(assets / "azpipe-demo.mp4"),
    ], check=True)
images[0].save(assets / "azpipe-demo.gif", save_all=True, append_images=images[1:],
               duration=[frame["seconds"] * 1000 for frame in frames], loop=0, optimize=True)
images[3].save(assets / "demo-review.png")
images[4].save(assets / "demo-monitoring.png")
print(f"PASS: {len(frames)} verified model frames; {sum(f['seconds'] for f in frames)} seconds; GIF + MP4")
