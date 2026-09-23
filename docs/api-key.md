# itch.io API key (paid games)

[← Back to the README](../README.md)

Free games download without any account or API key. You only need a key to
download **paid games you already own**.

The Settings screen shows `WORKING` (green) when a key is active and validated.

---

## Generating your API key

1. Log in to itch.io in a browser.
2. Go to <https://itch.io/user/settings/api-keys>.
3. Click **Generate new API key** and copy the key.

---

## Adding the key to Itch-io

### Option 1 — Built-in virtual keyboard (recommended)

Open **Settings** (press **Start** from any screen), navigate to **API Key**,
and press **A**. A virtual keyboard appears where you can type the key
directly on-device. Confirm with the **OK** key; Itch-io validates the key
immediately and shows `WORKING` on success.

To update an existing key, navigate to **API Key** in Settings and press **Y**
to open the virtual keyboard pre-filled with the current value.

### Option 2 — Browser-based file manager

With your device connected over USB, open
**[https://dashboard.loveretro.games/](https://dashboard.loveretro.games/)**
in a browser. Use the built-in file manager to navigate to
`.userdata/shared/Itch-io/config.json` and edit the `"api_key"` field directly
— no command line required, and only the fields you change are affected.

### Option 3 — SD card

1. Power off the device and remove the SD card.
2. Open `.userdata/shared/Itch-io/config.json` in a text editor.
3. Add or update the `"api_key"` field, leaving all other fields untouched.
4. Save, reinsert the SD card, and boot.

### Option 4 — ADB (pull → edit → push)

```sh
# Download the current config to your computer
adb pull /mnt/SDCARD/.userdata/shared/Itch-io/config.json config.json

# Open config.json in a text editor, add or update the "api_key" field:
#   "api_key": "YOUR_API_KEY_HERE"
# Leave all other fields unchanged, then push the file back:
adb push config.json /mnt/SDCARD/.userdata/shared/Itch-io/config.json
```

If you have never launched Itch-io and no config file exists yet, you can
create a minimal one:

```json
{
  "api_key": "YOUR_API_KEY_HERE"
}
```
