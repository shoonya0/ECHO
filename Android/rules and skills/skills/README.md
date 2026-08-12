# Skills Directory — ECHO Android

AI agent skills for the ECHO Android app. Each skill provides structured instructions that the AI follows when triggered by matching user intent.

## Layout

```
Android/rules and skills/skills/
├── README.md
├── error-handling-feedback/
│   └── SKILL.md
├── modular-development/
│   └── SKILL.md
├── build-verify-loop/
│   └── SKILL.md
├── compose-best-practices/
│   └── SKILL.md
├── websocket-integration/
│   └── SKILL.md
└── android-debugging/
    └── SKILL.md
```

## Skill Types

| Skill | Trigger |
|---|---|
| `error-handling-feedback` | "handle errors", "error handling", "fix crash", "crash loop", "map error to UI" |
| `modular-development` | "add feature", "new screen", "modular", "feature boundary", "clean architecture" |
| `build-verify-loop` | "build", "compile", "test", "verify", "deploy", "gradle build" |
| `compose-best-practices` | "composable", "UI", "state hoisting", "jetpack compose layout", "preview" |
| `websocket-integration` | "websocket", "real-time", "connect to server", "chat protocol", "reconnect" |
| `android-debugging` | "debug", "crash", "logcat", "adb", "StrictMode", "not working" |

## How Skills Work

1. AI loads skill descriptions at context start.
2. When user intent matches a description, the full `SKILL.md` is loaded.
3. Skills with checklists should be followed step-by-step.
4. Skills override generic knowledge with project-specific conventions.

## Frontmatter Format

Each `SKILL.md` starts with YAML frontmatter:

```yaml
---
name: skill-name
description: "When to trigger this skill. Be specific about user intent."
user-invocable: true
metadata:
  version: "1.0.0"
allowed-tools: Read Edit Write Bash Globbing Grep
---
```

## Adding New Skills

Create a new folder under `Android/rules and skills/skills/`:

```
Android/rules and skills/skills/my-new-skill/
  SKILL.md    # Required: YAML frontmatter + instructions