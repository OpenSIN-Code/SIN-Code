# Read-Only Invariant

The 6 read-only YouTube tools (`search`, `get_transcript`, `get_video_info`,
`get_channel_videos`, `get_channel_info`, `get_playlist`) are `allow` policy.
They never mutate anything on disk or on YouTube.

The 3 action tools (`download`, `clip`, `highlight_reel`) are `ask` policy (M4).
They write files to disk and require permission.

## Cookie safety

YouTube cookies (when configured) are stored at `~/.config/sin-youtube/cookies.json`
with chmod 600. They are secrets:

- NEVER log, echo, or commit cookie values
- NEVER paste cookie contents into chat
- Treat cookie file as a credential (like an API key)
- If cookies are exposed, revoke them by logging out of YouTube in a browser
