import os
import sys
import shutil
import subprocess
from PIL import Image, ImageDraw, ImageFont

WIDTH = 1200
HEIGHT = 600

# Color Palette: Catppuccin Mocha
BG = (30, 30, 46)             # #1E1E2E
TOP_BAR = (24, 24, 37)        # #181825
BORDER = (49, 50, 68)         # #313244
TEXT_FG = (205, 214, 244)     # #CDD6F4
TEXT_DIM = (147, 153, 178)    # #9399B2
PROMPT_DIR = (137, 180, 250)  # #89B4FA
PROMPT_CHAR = (166, 227, 161) # #A6E3A1

# Badges & Syntax
CYAN = (148, 226, 213)        # #94E2D5
GREEN = (166, 227, 161)       # #A6E3A1
RED = (243, 139, 168)         # #F38BA8
YELLOW = (249, 226, 175)      # #F9E2AF
PURPLE = (203, 166, 247)      # #CBA6F7
BLUE = (137, 180, 250)        # #89B4FA

PURPLE_BG = (90, 86, 224)     # #5A56E0
BLUE_BG = (46, 91, 255)       # #2E5BFF
DARK_TAB_BG = (34, 34, 34)    # #222222
CARD_BORDER = (68, 68, 68)    # #444444

# Fonts
FONT_PATH = "C:\\Windows\\Fonts\\CascadiaCode.ttf"
if not os.path.exists(FONT_PATH):
    FONT_PATH = "C:\\Windows\\Fonts\\consola.ttf"

FONT_MAIN = ImageFont.truetype(FONT_PATH, 14)
FONT_BOLD = ImageFont.truetype(FONT_PATH, 14)
FONT_TITLE = ImageFont.truetype(FONT_PATH, 14)
FONT_SMALL = ImageFont.truetype(FONT_PATH, 12)

def draw_window_frame():
    img = Image.new("RGB", (WIDTH, HEIGHT), BG)
    draw = ImageDraw.Draw(img)
    
    # Top bar
    draw.rectangle([(0, 0), (WIDTH, 36)], fill=TOP_BAR)
    draw.line([(0, 36), (WIDTH, 36)], fill=BORDER, width=1)
    
    # Traffic lights
    draw.ellipse([(18, 12), (28, 22)], fill=(255, 95, 86))
    draw.ellipse([(36, 12), (46, 22)], fill=(255, 189, 46))
    draw.ellipse([(54, 12), (64, 22)], fill=(39, 201, 63))
    
    # Title
    t = "prunedocker — demo — 120x35"
    bbox = FONT_SMALL.getbbox(t)
    tw = bbox[2] - bbox[0]
    draw.text(((WIDTH - tw) // 2, 11), t, fill=TEXT_DIM, font=FONT_SMALL)
    
    return img, draw

def draw_colored_line(draw, x, y, tokens):
    cx = x
    for item in tokens:
        text = item[0]
        fg = item[1]
        bg = item[2] if len(item) > 2 else None
        
        bbox = FONT_MAIN.getbbox(text)
        tw = bbox[2] - bbox[0]
        th = bbox[3] - bbox[1]
        
        if bg:
            pad_x = 4
            pad_y = 1
            draw.rectangle([(cx - pad_x, y - pad_y), (cx + tw + pad_x, y + 17 + pad_y)], fill=bg)
            
        draw.text((cx, y), text, fill=fg, font=FONT_MAIN)
        cx += tw
    return cx

frames = []
frame_durations = [] # in milliseconds

def add_frame(img, duration_ms):
    frames.append(img.copy())
    frame_durations.append(duration_ms)

# ==========================================
# SCENE 1: Shell & Type `prunedocker analyze`
# ==========================================
prompt_prefix = [
    ("❯ ", PROMPT_CHAR),
    ("dev@server", CYAN),
    (":", TEXT_DIM),
    ("~/projects/app", PROMPT_DIR),
    ("$ ", TEXT_DIM),
]

cmd1 = "prunedocker analyze"
for i in range(len(cmd1) + 1):
    base_img, draw = draw_window_frame()
    y = 52
    typed = cmd1[:i]
    tokens = list(prompt_prefix)
    tokens.append((typed, TEXT_FG))
    if i < len(cmd1):
        tokens.append(("▋", BLUE))
    draw_colored_line(draw, 24, y, tokens)
    add_frame(base_img, 70 if i < len(cmd1) else 400)

# ==========================================
# SCENE 2: Output of `prunedocker analyze`
# ==========================================
def render_analyze_screen(show_next_prompt=False, next_cmd=""):
    base_img, draw = draw_window_frame()
    y = 52
    # Prompt line
    p_tokens = list(prompt_prefix) + [(cmd1, TEXT_FG)]
    draw_colored_line(draw, 24, y, p_tokens)
    y += 24
    
    # Table header
    draw_colored_line(draw, 24, y, [("==========================================================================================", BORDER)])
    y += 18
    draw_colored_line(draw, 24, y, [(" PRUNEDOCKER LAYER DEPENDENCY & CACHE UTILITY REPORT", PURPLE)])
    y += 18
    draw_colored_line(draw, 24, y, [("==========================================================================================", BORDER)])
    y += 20
    draw_colored_line(draw, 24, y, [(" Total Images: 4 | Total Layers: 5 | Active Containers: 1", TEXT_DIM)])
    y += 22
    
    # Table columns
    col_header = " IMAGE ID     TAG                      SIZE       REUSE   SCORE    STATUS"
    draw_colored_line(draw, 24, y, [(col_header, TEXT_DIM)])
    y += 18
    draw_colored_line(draw, 24, y, [(" ---------------------------------------------------------------------------------------", BORDER)])
    y += 18
    
    # Rows
    draw_colored_line(draw, 24, y, [
        (" alpine319000 ", TEXT_FG),
        ("alpine:3.19              ", CYAN),
        ("10.0 MB    ", TEXT_DIM),
        ("4       ", TEXT_DIM),
        ("INF      ", PURPLE),
        ("ACTIVE_IN_USE", CYAN),
    ])
    y += 18
    draw_colored_line(draw, 24, y, [
        (" webapideps00 ", TEXT_FG),
        ("<none>                   ", TEXT_DIM),
        ("120.0 MB   ", TEXT_DIM),
        ("2       ", TEXT_DIM),
        ("0.02     ", GREEN),
        ("WARM_CACHE_KEPT", GREEN),
    ])
    y += 18
    draw_colored_line(draw, 24, y, [
        (" webapilatest ", TEXT_FG),
        ("web-api:latest           ", CYAN),
        ("245.0 MB   ", TEXT_DIM),
        ("1       ", TEXT_DIM),
        ("INF      ", PURPLE),
        ("ACTIVE_IN_USE", CYAN),
    ])
    y += 18
    draw_colored_line(draw, 24, y, [
        (" 71923058869b ", TEXT_FG),
        ("<none>                   ", TEXT_DIM),
        ("480.0 MB   ", TEXT_DIM),
        ("1       ", TEXT_DIM),
        ("0.00     ", RED),
        ("DEAD_LEAF_PRUNE", RED),
    ])
    y += 22
    
    # Volumes
    draw_colored_line(draw, 24, y, [(" Volumes:", TEXT_DIM)])
    y += 18
    draw_colored_line(draw, 24, y, [(" ---------------------------------------------------------------------------------------", BORDER)])
    y += 18
    draw_colored_line(draw, 24, y, [
        (" 4f2b1a3d9e8c                             ", TEXT_FG),
        ("150.0 MB     ", TEXT_DIM),
        ("ORPHAN_VOLUME_PRUNE", YELLOW),
    ])
    y += 18
    draw_colored_line(draw, 24, y, [
        (" postgres_production_data                 ", TEXT_FG),
        ("2400.0 MB    ", TEXT_DIM),
        ("NAMED_VOLUME_KEPT", BLUE),
    ])
    y += 22
    
    # Summary
    draw_colored_line(draw, 24, y, [(" Summary:", TEXT_DIM)])
    y += 18
    draw_colored_line(draw, 24, y, [(" ---------------------------------------------------------------------------------------", BORDER)])
    y += 18
    draw_colored_line(draw, 24, y, [
        ("  Protected Active:      2 images (255.00 MB)", CYAN)
    ])
    y += 18
    draw_colored_line(draw, 24, y, [
        ("  Warm Cache Kept:       1 images (120.00 MB)  --> PRESERVED FOR LIGHTNING REBUILDS", GREEN)
    ])
    y += 18
    draw_colored_line(draw, 24, y, [
        ("  Reclaimable Dead:      1 images, 1 volumes (630.00 MB reclaimable)", YELLOW)
    ])
    y += 18
    draw_colored_line(draw, 24, y, [("==========================================================================================", BORDER)])
    y += 24
    
    if show_next_prompt:
        next_tokens = list(prompt_prefix) + [(next_cmd, TEXT_FG)]
        if len(next_cmd) < len("prunedocker ui"):
            next_tokens.append(("▋", BLUE))
        draw_colored_line(draw, 24, y, next_tokens)
        
    return base_img

# Add analyze output frame (hold for 2.2s)
add_frame(render_analyze_screen(), 2200)

# ==========================================
# SCENE 3: Type `prunedocker ui`
# ==========================================
cmd2 = "prunedocker ui"
for i in range(len(cmd2) + 1):
    typed = cmd2[:i]
    f = render_analyze_screen(show_next_prompt=True, next_cmd=typed)
    add_frame(f, 80 if i < len(cmd2) else 500)

# ==========================================
# SCENE 4: Full Interactive Bubbletea TUI
# ==========================================
def render_tui(active_tab=0, dry_run=True, status_msg="Ready. Press [p] to prune, [d] to toggle dry-run, [tab] to switch tabs.", status_color=YELLOW, selected_row=0):
    base_img, draw = draw_window_frame()
    y = 50
    
    # 1. Header
    mode_text = "[DRY-RUN ON]" if dry_run else "[LIVE MODE]"
    mode_color = GREEN if dry_run else RED
    draw_colored_line(draw, 24, y, [
        (" PRUNEDOCKER ", (255, 255, 255), PURPLE_BG),
        ("  ", TEXT_FG),
        ("Intelligent Layer-Preserving Cache Optimizer", TEXT_DIM),
        ("   ", TEXT_FG),
        (mode_text, mode_color),
    ])
    y += 32
    
    # 2. Card
    card_x = 24
    card_w = WIDTH - 48
    card_h = 60
    draw.rounded_rectangle([(card_x, y), (card_x + card_w, y + card_h)], radius=6, outline=CARD_BORDER, width=1)
    
    draw_colored_line(draw, card_x + 14, y + 10, [
        ("Protected: ", TEXT_DIM),
        ("[ACTIVE IN USE]", CYAN),
        (" (2 img, 255.0 MB)  |  ", TEXT_DIM),
        ("[WARM CACHE KEPT]", GREEN),
        (" (1 img, 120.0 MB)", TEXT_DIM),
    ])
    draw_colored_line(draw, card_x + 14, y + 34, [
        ("Candidates: ", TEXT_DIM),
        ("[DEAD LEAF PRUNE]", RED),
        (" (1 dead img)  |  ", TEXT_DIM),
        ("[ORPHAN VOL PRUNE]", YELLOW),
        (" (1 orphan vol)  |  Reclaimable: ", TEXT_DIM),
        ("630.0 MB", YELLOW),
    ])
    y += card_h + 16
    
    # 3. Tabs
    t0_bg = BLUE_BG if active_tab == 0 else DARK_TAB_BG
    t0_fg = (255, 255, 255) if active_tab == 0 else TEXT_DIM
    t1_bg = BLUE_BG if active_tab == 1 else DARK_TAB_BG
    t1_fg = (255, 255, 255) if active_tab == 1 else TEXT_DIM
    t2_bg = BLUE_BG if active_tab == 2 else DARK_TAB_BG
    t2_fg = (255, 255, 255) if active_tab == 2 else TEXT_DIM
    
    draw_colored_line(draw, 24, y, [
        (" 1. Images & Layers ", t0_fg, t0_bg),
        ("  ", TEXT_FG),
        (" 2. Volumes ", t1_fg, t1_bg),
        ("  ", TEXT_FG),
        (" 3. Prune Plan ", t2_fg, t2_bg),
    ])
    y += 32
    
    # 4. Tab Content
    if active_tab == 0:
        draw_colored_line(draw, 24, y, [("  IMAGE ID       TAG / REPO                SIZE        REUSE   SCORE    STATUS", TEXT_DIM)])
        y += 18
        draw_colored_line(draw, 24, y, [("  ------------------------------------------------------------------------------------------", BORDER)])
        y += 18
        
        rows = [
            ("> alpine319000   ", "alpine:3.19               ", "10.0 MB     ", "4       ", "INF      ", "[ACTIVE IN USE]", CYAN),
            ("  webapideps00   ", "<none>                    ", "120.0 MB    ", "2       ", "0.02     ", "[WARM CACHE KEPT]", GREEN),
            ("  webapilatest   ", "web-api:latest            ", "245.0 MB    ", "1       ", "INF      ", "[ACTIVE IN USE]", CYAN),
            ("  71923058869b   ", "<none>                    ", "480.0 MB    ", "1       ", "0.00     ", "[DEAD LEAF PRUNE]", RED),
        ]
        for idx, r in enumerate(rows):
            cur = "> " if idx == selected_row else "  "
            draw_colored_line(draw, 24, y, [
                (cur + r[0][2:], (255, 255, 255) if idx == selected_row else TEXT_FG),
                (r[1], CYAN if "<none>" not in r[1] else TEXT_DIM),
                (r[2], TEXT_DIM),
                (r[3], TEXT_DIM),
                (r[4], PURPLE if "INF" in r[4] else (GREEN if "0.02" in r[4] else RED)),
                (r[5], r[6]),
            ])
            y += 20
            
    elif active_tab == 1:
        draw_colored_line(draw, 24, y, [("  VOLUME NAME                                      SIZE        STATUS", TEXT_DIM)])
        y += 18
        draw_colored_line(draw, 24, y, [("  ------------------------------------------------------------------------------------------", BORDER)])
        y += 18
        
        vols = [
            ("> 4f2b1a3d9e8c7b6a5f4e3d2c1b0a9f8e7d...            ", "150.0 MB    ", "[ORPHAN VOL PRUNE]", YELLOW),
            ("  postgres_production_data                         ", "2400.0 MB   ", "[SAVED VOLUME]", BLUE),
        ]
        for idx, v in enumerate(vols):
            draw_colored_line(draw, 24, y, [
                (v[0], (255, 255, 255) if idx == 0 else TEXT_FG),
                (v[1], TEXT_DIM),
                (v[2], v[3]),
            ])
            y += 20
            
    elif active_tab == 2:
        draw_colored_line(draw, 24, y, [("  Targets to Prune: 1 Dead Images, 1 Orphan Volumes", YELLOW)])
        y += 18
        draw_colored_line(draw, 24, y, [("  Total Projected Disk Reclamation: 630.00 MB", (255, 255, 255))])
        y += 18
        draw_colored_line(draw, 24, y, [("  Warm Build Cache Strictly Preserved: 120.00 MB (1 images protected)", GREEN)])
        y += 18
        draw_colored_line(draw, 24, y, [("  ------------------------------------------------------------------------------------------", BORDER)])
        y += 18
        draw_colored_line(draw, 24, y, [("  - [IMAGE]  71923058869b (480.0 MB) - Zero child dependencies & score 0.00", RED)])
        y += 18
        draw_colored_line(draw, 24, y, [("  - [VOLUME] 4f2b1a3d9e8c (150.0 MB) - Unattached anonymous volume", YELLOW)])
        y += 20

    # 5. Status line
    y_status = HEIGHT - 65
    draw_colored_line(draw, 24, y_status, [
        ("Status: ", status_color),
        (status_msg, status_color),
    ])
    
    # 6. Help footer
    y_help = HEIGHT - 35
    draw_colored_line(draw, 24, y_help, [
        ("[Tab]", CYAN), (" Switch View   ", TEXT_DIM),
        ("[p]", CYAN), (" Prune   ", TEXT_DIM),
        ("[d]", CYAN), (" Toggle Dry-Run   ", TEXT_DIM),
        ("[r]", CYAN), (" Rescan   ", TEXT_DIM),
        ("[j/k]", CYAN), (" Scroll   ", TEXT_DIM),
        ("[q]", CYAN), (" Quit", TEXT_DIM),
    ])
    
    return base_img

# Step 1 in TUI: Initial state (Images tab)
add_frame(render_tui(active_tab=0, dry_run=True), 1800)

# Step 2: Toggle Dry-Run (press 'd') -> LIVE MODE
add_frame(render_tui(active_tab=0, dry_run=False, status_msg="Mode changed: LIVE PRUNING ENABLED (Real deletions will occur!)", status_color=RED), 1400)

# Step 3: Toggle back to DRY-RUN (press 'd')
add_frame(render_tui(active_tab=0, dry_run=True, status_msg="Mode changed: DRY-RUN ENABLED (Safe, zero deletions)", status_color=GREEN), 1200)

# Step 4: Scroll down (press 'j')
add_frame(render_tui(active_tab=0, dry_run=True, selected_row=1), 800)
add_frame(render_tui(active_tab=0, dry_run=True, selected_row=3), 1000)

# Step 5: Switch tab to Volumes (press 'Tab')
add_frame(render_tui(active_tab=1, dry_run=True, status_msg="Viewing Volumes (Tab 2/3)"), 1500)

# Step 6: Switch tab to Prune Plan (press 'Tab')
add_frame(render_tui(active_tab=2, dry_run=True, status_msg="Viewing Prune Plan (Tab 3/3)"), 1800)

# Step 7: Trigger Prune (press 'p')
add_frame(render_tui(active_tab=2, dry_run=True, status_msg="Confirm DRY-RUN simulation? Press [y] to confirm, [n] to cancel.", status_color=YELLOW), 1600)

# Step 8: Confirm Prune (press 'y')
add_frame(render_tui(active_tab=2, dry_run=True, status_msg="[DRY-RUN COMPLETE] Reclaimed 630.00 MB across 1 images and 1 volumes. Warm cache untouched!", status_color=GREEN), 2800)

print(f"Rendered {len(frames)} frames. Compiling GIF...")

# Output directory
output_gif = "assets/demo.gif"
temp_frames_dir = "assets/temp_frames"
if os.path.exists(temp_frames_dir):
    shutil.rmtree(temp_frames_dir)
os.makedirs(temp_frames_dir, exist_ok=True)

# Save temporary PNG frames
concat_file = os.path.join(temp_frames_dir, "input.txt")
with open(concat_file, "w") as f:
    for idx, (frame, dur) in enumerate(zip(frames, frame_durations)):
        frame_path = os.path.join(temp_frames_dir, f"frame_{idx:04d}.png")
        frame.save(frame_path)
        # dur is in ms, ffmpeg duration is in seconds
        dur_sec = dur / 1000.0
        f.write(f"file 'frame_{idx:04d}.png'\n")
        f.write(f"duration {dur_sec:.3f}\n")
    # Duplicate last file for concat demuxer
    f.write(f"file 'frame_{len(frames)-1:04d}.png'\n")

# Use FFmpeg with optimal 2-pass palette generation for ultra crisp quality
cmd = [
    "ffmpeg", "-y",
    "-f", "concat", "-safe", "0", "-i", concat_file,
    "-vf", "split[s0][s1];[s0]palettegen=stats_mode=full[p];[s1][p]paletteuse=dither=bayer:bayer_scale=3",
    output_gif
]

result = subprocess.run(cmd, capture_output=True, text=True)
if result.returncode != 0:
    print("FFmpeg error:", result.stderr)
    # Fallback to PIL native save
    frames[0].save(
        output_gif,
        save_all=True,
        append_images=frames[1:],
        duration=frame_durations,
        loop=0,
        optimize=True
    )
    print("Saved using PIL fallback.")
else:
    print(f"Successfully generated {output_gif} via FFmpeg!")

# Cleanup temp frames
shutil.rmtree(temp_frames_dir, ignore_errors=True)
print("GIF creation complete.")
