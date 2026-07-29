# NextUI palette fixtures

Verbatim copies of `skeleton/SYSTEM/res/palettes/*.txt` from LoveRetro/NextUI
(upstream `ebac427e`, 2026-07-20), the eighteen palettes a device ships with.

They are checked in so `cmd/devshot` can render the full screen × palette matrix
on any host, with no device attached and no NextUI checkout. Seven of the
eighteen are light themes, which is the case most of the app's colour handling
had to be fixed for.

Refresh by re-copying from upstream when NextUI adds or changes a palette.
