---
name: build-verify-loop
description: "Code → build → test → fix iteration loop for the ECHO Android app. Triggers on 'build', 'compile', 'test', 'verify', 'deploy', 'gradle build', or when code changes need verification."
user-invocable: true
metadata:
  version: "1.0.0"
allowed-tools: Read Edit Write Bash Globbing Grep
---

**Persona:** You are a build engineer. Every code change must pass `assembleDebug` and unit tests before being considered done. You never skip verification.

# Build-Verify-Loop

Every code change follows this cycle:

```
Write code → ./gradlew assembleDebug → Fix errors → Rebuild
       → ./gradlew testDebugUnitTest → Fix failures → Retest
       → ./gradlew lintDebug → Fix warnings → Relint
       → Done
```

## Step 1: Build

```bash
cd /home/user/projects/ECHO/Android && ./gradlew assembleDebug 2>&1
```

### If build fails:
- Read the error output carefully
- Identify the file and line number
- Fix the specific issue (missing import, type mismatch, syntax error)
- Rebuild — do NOT skip to tests

Common build errors:
| Error | Likely cause |
|---|---|
| `Unresolved reference` | Missing import or dependency |
| `Type mismatch` | Wrong type passed to function |
| `Cannot access class` | Missing `@InstallIn` on Hilt module |
| `Duplicate class` | Conflicting dependency versions |

## Step 2: Unit Tests

```bash
cd /home/user/projects/ECHO/Android && ./gradlew testDebugUnitTest 2>&1
```

### If tests fail:
- Read the assertion failure message
- Identify which test class and method failed
- Check if the test expectation is correct OR if the code is wrong
- Fix and re-run

Do NOT modify test assertions to match broken code unless the requirements changed.

## Step 3: Lint

```bash
cd /home/user/projects/ECHO/Android && ./gradlew lintDebug 2>&1
```

Address warnings before committing. Ignored warnings must have a comment explaining why.

## Step 4: Instrumentation Tests (optional, requires emulator)

```bash
cd /home/user/projects/ECHO/Android && ./gradlew connectedAndroidTest 2>&1
```

Run before merging to main. Only if an emulator or device is connected.

## Failure Handling

Apply the 3-strike protocol from `error-handling-feedback`:

1. **Build error:** Read error → fix → rebuild
2. **Same error persists:** Try alternative fix → rebuild
3. **Still failing:** Check for upstream issues (gradle cache? dependency conflict? wrong JDK version?)

## Common Gradle Commands

| Command | Purpose |
|---|---|
| `./gradlew assembleDebug` | Build debug APK |
| `./gradlew assembleRelease` | Build release APK |
| `./gradlew testDebugUnitTest` | Run unit tests |
| `./gradlew lintDebug` | Run lint checks |
| `./gradlew connectedAndroidTest` | Run instrumentation tests |
| `./gradlew clean` | Clean build artifacts |
| `./gradlew dependencies` | Show dependency tree |

## Build Verification Checklist

After every code change:
- [ ] `assembleDebug` passes
- [ ] `testDebugUnitTest` passes (all tests green)
- [ ] `lintDebug` shows no new errors
- [ ] No `printStackTrace()` or `TODO()` left in production code
- [ ] No hardcoded strings that should be resources (use `@Composable` preview data instead)

## Cross-References

- → `Android/rules and skills/skills/error-handling-feedback/SKILL.md` — 3-strike error protocol
- → `Android/rules and skills/rules/gradle-build.md` — Gradle configuration
- → `Android/rules and skills/rules/testing.md` — Testing conventions