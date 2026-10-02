---
name: min
description: Keep answers tight - one line by default, bullets only with the `bullets` arg, code only with the `code` arg, no jargon. Use whenever the user invokes /min, asks for "concise" answers, or otherwise nudges for short replies.
---

# min

Keep answers tight. The user has limited reading time and is fluent in the topic.

## Scope

- If the user's message contains a question or request alongside `/min`, answer that in concise form.
- If `/min` is invoked alone (no clarifying content), redo the previous assistant turn in concise form.

## Arguments

- `bullets` - switch to bullets, each 5-9 words max, prefer shorter. No prose around the list.
- `code` - code allowed, max 3 lines per concept unless the user specifies otherwise. No surrounding commentary unless asked.
- (no arg) - one line. Nothing more.

Args may be combined (e.g. `/min bullets code`).

## Always

- No jargon, no buzzwords, no non-standard phrases.
- No filler ("Sure!", "Of course", "Great question").
- No repeating the question back.
- No "let me know if..." closings.
- No acknowledging the skill itself ("Acknowledged", "Got it", "Will do"). Just answer the user's actual question in concise form. The invocation alone is enough; never spend a turn confirming you heard it.
