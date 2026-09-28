"""Render offline model frames as a GIF and a shareable MP4. Requires Pillow/FFmpeg."""
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import unicodedata

from PIL import Image, ImageDraw, ImageFont


def terminal_color(index):
    if index < 16:
        return (
            "#000000", "#800000", "#008000", "#808000",
            "#000080", "#800080", "#008080", "#c0c0c0",
            "#808080", "#ff0000", "#00ff00", "#ffff00",
            "#0000ff", "#ff00ff", "#00ffff", "#ffffff",
        )[index]
    if index >= 232:
        level = 8 + 10 * (index - 232)
        return f"#{level:02x}{level:02x}{level:02x}"
    index -= 16
    levels = (0, 95, 135, 175, 215, 255)
    return "#" + "".join(f"{levels[n]:02x}" for n in (index // 36, index // 6 % 6, index % 6))


def styled_spans(line):
    """Read the SGR colours emitted by the model instead of guessing from its text."""
    foreground, background, bold = "#e8e9e8", "#101416", False
    for part in re.split(r"(\x1b\[[0-9;]*m|\x1b\].*?(?:\x07|\x1b\\))", line):
        if part.startswith("\x1b]"):
            continue
        if not part.startswith("\x1b["):
            yield part, foreground, background, bold
            continue
        codes = [int(value or 0) for value in part[2:-1].split(";")]
        i = 0
        while i < len(codes):
            code = codes[i]
            if code == 0:
                foreground, background, bold = "#e8e9e8", "#101416", False
            elif code in (1, 22):
                bold = code == 1
            elif code == 39:
                foreground = "#e8e9e8"
            elif code == 49:
                background = "#101416"
            elif code in (38, 48) and codes[i + 1:i + 2] == [5]:
                color = terminal_color(codes[i + 2])
                if code == 38:
                    foreground = color
                else:
                    background = color
                i += 2
            elif 30 <= code <= 37 or 90 <= code <= 97:
                foreground = terminal_color(code - 30 if code < 90 else code - 82)
            elif 40 <= code <= 47 or 100 <= code <= 107:
                background = terminal_color(code - 40 if code < 100 else code - 92)
            i += 1


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
bold_font = ImageFont.truetype(font_path, 18, index=1) if font_path.endswith(".ttc") else font
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
        for row, line in enumerate(frame.get("ansi", frame["view"]).splitlines()):
            col = 0
            for text, foreground, background, bold in styled_spans(line):
                for char in text:
                    cells = 0 if unicodedata.combining(char) else (2 if unicodedata.east_asian_width(char) in "WF" else 1)
                    x, y = 32 + col * 12, 84 + row * 23
                    if cells:
                        draw.rectangle((x, y, x + cells * 12 - 1, y + 22), fill=background)
                    # Menlo Bold lacks some terminal line-drawing glyphs.
                    glyph_font = bold_font if bold and char.isalnum() else font
                    draw.text((x, y), char, font=glyph_font, fill=foreground)
                    col += cells
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
images[0].save(assets / "demo-catalog.png")
print(f"PASS: {len(frames)} verified model frames; {sum(f['seconds'] for f in frames)} seconds; GIF + MP4")
