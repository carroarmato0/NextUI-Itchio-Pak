# Use Game Title as Filename

## The problem this solves

Games on itch.io are published by independent developers who can name their downloadable
files anything they like. Some are descriptive (`Solastra v1.2.gbc`). Others are not —
Doomslinger Dungeon, for example, offers its files simply as **"Game Boy ROM"** and
**"Old Game Jam Version"**, with no indication of which game they belong to.

Without this feature, those vague names end up on your device exactly as-is. Your Game Boy
library might look like this:

```
Game Boy (GB)/
  Game Boy ROM.gb
  Old Game Jam Version.gb
  Solastra v1.2.gbc
```

Cover art, save files, and emulator history all use the filename as a key. Vague names make
it hard to tell what you have, and harder still to keep things organised when you have
multiple games from different developers.

---

## What this feature does

When **"Use game title as filename"** is turned on, the app automatically renames each
downloaded ROM to match the game's title on itch.io, using the file extension you chose.

The same library now looks like this:

```
Game Boy (GB)/
  Doomslinger Dungeon.gb
  Solastra.gbc
```

The original filename from itch.io is remembered internally so the app can still detect
when a new version is available — you won't miss updates just because the file was renamed.

---

## Default behaviour

The feature is **on by default**. You don't need to configure anything. When you download a
game, the app silently applies the rename and shows you the final filename at the end of the
download:

```
Download complete
Saved as: Doomslinger Dungeon.gb
Location: Game Boy (GB)/
```

---

## Downloading an update

If a new version of a game you already have appears on itch.io, the app will ask before
applying the rename again:

```
Rename to game title?

  New file from itch.io : Game Boy ROM v2.gb
  Will be saved as      : Doomslinger Dungeon.gb

  [A] Confirm    [B] Keep original name
```

- Press **A** to save the update under the same friendly name as before.
- Press **B** to keep the original filename from itch.io for this download only.

Pressing B here does not permanently change any setting — it is a one-time choice for that
specific download.

---

## Turning it off globally

Open **Settings** and find the toggle:

```
Use game title as filename   [ON]
```

Switch it off and all future downloads will be saved under whatever filename itch.io
provides, unchanged. Games you have already downloaded are not affected — they stay as they
are.

> To rename existing downloads back to their original filenames, use **Manage Downloads**
> (see below).

---

## Turning it off for a single game

Sometimes a developer's filename already says exactly what it is, or you have a specific
reason to keep the original name. You can disable the rename for individual games without
changing the global setting.

Go to **Manage Downloads**, highlight the game, and press **Y**:

```
Doomslinger Dungeon

  [A] Delete download
  [Y] Disable title filename
  [B] Back
```

The app will immediately rename the file back to its original name from itch.io. From that
point on, any new downloads or updates for that game will also use the original name.

To re-enable it for the same game, go back to Manage Downloads, press **Y** again, and the
option will now read **"Enable title filename"**.

> The per-game option is only available when the global setting is on.

---

## Save file safety

Your save games are tied to the ROM filename. Renaming a ROM without renaming its save
means the emulator can no longer find the save when you launch the game.

The app detects this situation automatically. Any time a rename would affect a file that
has a save on disk, you will be asked first:

```
Save file detected

  A save file exists for this game:
  GB/Doomslinger Dungeon.gb.sav

  Rename it to match the new ROM name?
  If you skip this, your save will not
  load until renamed manually.

  [A] Rename save   [B] Skip
```

- **Rename save (A):** the save file is renamed alongside the ROM. Your progress is
  preserved and the emulator finds it automatically next time you launch.
- **Skip (B):** only the ROM is renamed. Your save file stays where it is under the old
  name. You can rename it yourself later using your device's file manager.

### If a save already exists at the new name

In a rare case — for example if you downloaded a different version of the same game that
has already built up its own save — the app will warn you before overwriting:

```
A save already exists at the new path.
Overwrite it?   [A] Yes   [B] Cancel
```

- **Yes (A):** the existing save at the new path is replaced. Use this only if you are sure
  you no longer need it.
- **Cancel (B):** the entire rename is cancelled. Nothing is changed. This is the safe
  option if you are not sure which save is the one you want to keep.

---

## Summary

| Situation | What happens |
|-----------|-------------|
| Fresh download, feature on | ROM silently saved under game title |
| Fresh download, feature off (global) | ROM saved under itch.io filename |
| Fresh download, feature off (per-game) | ROM saved under itch.io filename |
| Update download, feature on | App asks you to confirm the rename |
| Enabling title filename for existing game, no save | ROM and cover art renamed immediately |
| Enabling title filename for existing game, save present | App asks whether to rename the save too |
| Disabling title filename for existing game | ROM and cover art renamed back; save prompt if needed |
| Global setting toggled | Applies to future downloads only; existing files unchanged |
