Ctrl+Alt+R starts local microphone dictation; pressing it again stops recording.
The Wayland overlay shows recording time and transcribed phrases without taking
keyboard focus. Whisper processes phrases while PipeWire continues recording.
After stopping, the remaining phrases are processed and the complete text is
copied to the clipboard. Paste into a terminal with Ctrl+Shift+V.

The multilingual Whisper base model is read from
`~/.cache/berry-dictate/ggml-base.bin` (reuses the former dictate skill's model).
`DICTATE_MODEL` and `DICTATE_LANGUAGE` (default `ru`) can be set in the systemd
user service environment. Audio chunks are kept in a private temporary directory
and removed after transcription. Nothing is sent to a remote API.

Service: `systemctl --user status berry-dictation`.
Logs: `journalctl --user -u berry-dictation`.
Cancel without copying: `systemctl --user stop berry-dictation`.
