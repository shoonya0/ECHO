---
name: android-debugging
description: "Debug Android app issues using logcat, adb, StrictMode, and structured troubleshooting. Triggers on 'debug', 'crash', 'logcat', 'adb', 'StrictMode', 'not working', or when diagnosing runtime issues."
user-invocable: true
metadata:
  version: "1.0.0"
allowed-tools: Read Edit Write Bash Globbing Grep
---

**Persona:** You are an Android debugging specialist. Every crash has a root cause. Use logcat, adb, and Timber tags to trace the issue, then fix it systematically.

# Android Debugging Checklist

## Step 1: Gather Information

Before touching code, collect:

- [ ] **What broke?** Crash? ANR? Wrong UI? No data?
- [ ] **When did it break?** After what change?
- [ ] **What does logcat say?** Filter by Timber tag or error level.
- [ ] **Reproduction steps?** Can you reproduce it consistently?

## Step 2: Logcat Commands

```bash
# Filter by Timber tag
adb logcat -s "ChatVM" "WebSocketClient" "AuthRepo"

# Filter by error level
adb logcat *:E

# Filter by package
adb logcat --pid=$(adb shell pidof com.Shoonya.Echo)

# Clear and watch
adb logcat -c && adb logcat

# Save to file for analysis
adb logcat -d > crash_log.txt
```

## Step 3: Structured Logging with Timber

All debug logging must use Timber with tags:

```kotlin
// Tag by feature + layer
Timber.tag("ChatVM").d("state: %s", state.value)
Timber.tag("ChatRepo").e(err, "failed to fetch chats")
Timber.tag("WS").i("connected to %s", BuildConfig.WS_BASE_URL)
Timber.tag("AuthInterceptor").d("token present: %b", token != null)
```

Tag conventions:
| Tag | Where |
|---|---|
| `ChatVM`, `AuthVM`, etc. | ViewModels |
| `ChatRepo`, `AuthRepo` | Repositories |
| `WS` | WebSocket client |
| `API` | Retrofit calls |
| `AuthInterceptor` | Auth interceptor |
| `SafeScreen` | Crash loop protection |

## Step 4: Common Debugging Scenarios

### Network not working on emulator
```bash
# Emulator localhost = 10.0.2.2, not 127.0.0.1
# Verify with:
adb shell ping 10.0.2.2

# Check if server is running:
curl http://10.0.2.2:8080/echo/v1/chats/hub/stats
```

### WebSocket not connecting
- [ ] Check token: is `SecureTokenStore.getAccessToken()` returning null?
- [ ] Check URL: emulator uses `ws://10.0.2.2:8080`, not `ws://localhost:8080`
- [ ] Check server: is the Go backend running?
- [ ] Check network security config: cleartext allowed for debug?

### Composable not recomposing
- [ ] Is the state `StateFlow`? MutableList won't trigger recomposition.
- [ ] Are you using `collectAsStateWithLifecycle()`?
- [ ] Is the key stable? LazyColumn with index as key skips updates.

### Hilt injection failing
- [ ] `@HiltAndroidApp` on Application?
- [ ] `@AndroidEntryPoint` on Activity?
- [ ] `@HiltViewModel` on ViewModel?
- [ ] Module annotated with `@InstallIn(SingletonComponent::class)`?

### Build failing with "Duplicate class"
```
# Check dependency tree:
./gradlew :app:dependencies --configuration debugRuntimeClasspath | grep "duplicate_class_name"

# Common fix: exclude transitive dependency
implementation("lib") {
    exclude(group = "conflicting.group", module = "conflicting-module")
}
```

## Step 5: StrictMode

Enable in debug builds to catch main-thread I/O and leaks:

```kotlin
// EchoApplication.kt
if (BuildConfig.DEBUG) {
    StrictMode.setThreadPolicy(
        StrictMode.ThreadPolicy.Builder()
            .detectDiskReads()
            .detectDiskWrites()
            .detectNetwork()
            .penaltyLog()
            .build()
    )
    StrictMode.setVmPolicy(
        StrictMode.VmPolicy.Builder()
            .detectLeakedSqlLiteObjects()
            .detectLeakedClosableObjects()
            .detectActivityLeaks()
            .penaltyLog()
            .build()
    )
}
```

## Step 6: Memory Leaks

```bash
# Take heap dump
adb shell am dumpheap com.Shoonya.Echo /data/local/tmp/heap.hprof
adb pull /data/local/tmp/heap.hprof

# Check for retained Activities (common ViewModel leak)
# ViewModel holding Context or View reference = leak
```

```kotlin
// ✗ BAD — leaks Context
class ChatViewModel(private val context: Context) : ViewModel()

// ✓ GOOD — use AndroidViewModel or inject Application context
class ChatViewModel(
    private val repo: ChatRepository,
    @ApplicationContext private val appContext: Context  // OK
) : AndroidViewModel(appContext)
```

## Step 7: Crash Investigation Protocol

1. **Read the stack trace** — topmost line is the crash location.
2. **Is it a NullPointerException?** Check null safety — was a `!!` or lateinit access?
3. **Is it an IndexOutOfBoundsException?** Check list access with `getOrNull()`.
4. **Is it a NetworkOnMainThreadException?** Missing `withContext(Dispatchers.IO)`.
5. **Is it an IllegalStateException from Compose?** State read outside composition.

## Step 8: Verify the Fix

After applying a fix:
- [ ] `./gradlew assembleDebug` passes
- [ ] Run the debug flow again — does it still crash?
- [ ] Check logcat for Timber warnings/errors — any new issues?
- [ ] Run `./gradlew testDebugUnitTest` — did the fix break any tests?

## Cross-References

- → `Android/rules and skills/skills/error-handling-feedback/SKILL.md` — 3-strike protocol
- → `Android/rules and skills/skills/build-verify-loop/SKILL.md` — Build and test cycle
- → `Android/rules and skills/rules/security.md` — No sensitive data in logs
- → `Android/rules and skills/rules/coroutines-flow.md` — Dispatcher rules