# VHS Tape for PruneDocker Terminal Demo
# Documentation: https://github.com/charmbracelet/vhs

Output assets/demo.gif

Set FontSize 14
Set Width 1200
Set Height 600
Set Padding 20
Set Theme "Catppuccin Mocha"

# Step 1: Analyze Docker Layers
Type "prunedocker analyze"
Sleep 500ms
Enter
Sleep 3s

# Step 2: Launch Interactive TUI Dashboard
Type "prunedocker ui"
Sleep 500ms
Enter
Sleep 3s

# Step 3: Toggle Dry-Run Mode
Type "d"
Sleep 1s

# Step 4: Navigate Tabs
Type @500ms "	"
Sleep 1s
Type @500ms "	"
Sleep 1s

# Step 5: Trigger Selective Pruning Plan
Type "p"
Sleep 1500ms

# Step 6: Confirm Prune
Type "y"
Sleep 3s

# Step 7: Exit Cleanly
Type "q"
Sleep 1s
