# Security — ECHO Android

## Rule

1. **JWT tokens** are stored in EncryptedSharedPreferences — never in plain SharedPreferences, DataStore, or passed via Intent extras.
2. **No secrets in code.** API URLs, keys, and config values come from `BuildConfig` or a local properties file that is gitignored.
3. **HTTPS only** for release builds. `android:usesCleartextTraffic="false"` in the release manifest.
4. **ProGuard/R8** is enabled for release builds to obfuscate and shrink.
5. **Sensitive fields** are excluded from logs — tokens, passwords, PII are never passed to Timber.

## Why

- Plain SharedPreferences can be read by rooted devices and other apps on older Android versions. EncryptedSharedPreferences uses AES-256 + Android Keystore.
- A JWT token in plain storage is effectively a password — stealing it gives full access to the user's ECHO account.
- ProGuard obfuscation makes reverse engineering significantly harder.

## How to apply

### 1. EncryptedSharedPreferences for tokens

```kotlin
// core/data/local/SecureTokenStore.kt
@Singleton
class SecureTokenStore @Inject constructor(
    @ApplicationContext private val context: Context
) {
    private val masterKey = MasterKey.Builder(context)
        .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
        .build()

    private val prefs = EncryptedSharedPreferences.create(
        context,
        "echo_secure_prefs",
        masterKey,
        EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
        EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM
    )

    fun saveAccessToken(token: String) {
        prefs.edit().putString("jwt_access_token", token).apply()
    }

    fun getAccessToken(): String? {
        return prefs.getString("jwt_access_token", null)
    }

    fun clearTokens() {
        prefs.edit().remove("jwt_access_token").apply()
    }
}
```

### 2. No secrets in version control

```
# .gitignore
local.properties
*.keystore
*.jks
google-services.json  # If using Firebase, fetch from CI
```

```kotlin
// local.properties (gitignored) — read via Gradle
// API_BASE_URL=https://echo.example.com

// In androidbuild.gradle.kts:
val localProperties = Properties().apply {
    rootProject.file("local.properties").takeIf { it.exists() }?.inputStream()?.use { load(it) }
}
buildTypes {
    release {
        buildConfigField("String", "API_BASE_URL", "\"${localProperties["API_BASE_URL"] ?: "https://echo.example.com"}\"")
    }
}
```

### 3. Network security — cleartext disabled in release

```xml
<!-- AndroidManifest.xml -->
<application
    android:usesCleartextTraffic="false"  <!-- release default -->
    android:networkSecurityConfig="@xml/network_security_config">
```

```xml
<!-- res/xml/network_security_config.xml -->
<network-security-config>
    <domain-config cleartextTrafficPermitted="false">
        <domain includeSubdomains="true">echo.example.com</domain>
        <trust-anchors>
            <certificates src="system" />
        </trust-anchors>
    </domain-config>
    <!-- Debug: allow localhost for emulator -->
    <domain-config cleartextTrafficPermitted="true">
        <domain includeSubdomains="false">10.0.2.2</domain>
    </domain-config>
</network-security-config>
```

### 4. Certificate pinning (OkHttp)

```kotlin
// core/data/remote/CertificatePinner.kt
val certificatePinner = CertificatePinner.Builder()
    .add(
        "echo.example.com",
        "sha256/AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="  // Replace with actual pin
    )
    .build()

val okHttpClient = OkHttpClient.Builder()
    .certificatePinner(certificatePinner)
    .build()
```

### 5. No sensitive data in logs

```kotlin
// ✓ GOOD — log only non-sensitive info
Timber.tag("AuthRepo").d("login attempt for: %s", email.sanitize())
Timber.tag("WS").d("sending frame type=%s, requestId=%s", command.type, command.requestId)

// ✗ BAD — logging tokens or passwords
Timber.tag("AuthRepo").d("token: %s", accessToken)   // NEVER
Timber.tag("AuthRepo").d("password: %s", password)    // NEVER
Timber.tag("ChatVM").d("message content: %s", message.content)  // NEVER log user messages
```

### 6. ProGuard rules

```properties
# proguard-rules.pro
# Keep serializable WebSocket models (used by kotlinx.serialization)
-keepattributes *Annotation*, InnerClasses
-dontnote kotlinx.serialization.AnnotationsKt

-keepclassmembers class kotlinx.serialization.json.** {
    *** Companion;
}
-keepclasseswithmembers class kotlinx.serialization.json.** {
    kotlinx.serialization.KSerializer serializer(...);
}

-keep,includedescriptorclasses class com.shoonya.echo.**$$serializer { *; }
-keepclassmembers class com.shoonya.echo.** {
    *** Companion;
}
-keepclasseswithmembers class com.shoonya.echo.** {
    kotlinx.serialization.KSerializer serializer(...);
}
```

### 7. Root detection (optional, for sensitive operations)

```kotlin
// core/util/RootDetector.kt
object RootDetector {
    fun isDeviceRooted(): Boolean {
        val paths = listOf(
            "/system/app/Superuser.apk",
            "/sbin/su",
            "/system/bin/su",
            "/system/xbin/su",
            "/data/local/xbin/su",
            "/data/local/bin/su",
            "/system/sd/xbin/su",
            "/system/bin/failsafe/su",
            "/data/local/su",
        )
        return paths.any { File(it).exists() }
    }
}
```

### 8. Tapjacking protection

```kotlin
// In sensitive screens (login, payment):
@Composable
fun LoginScreen() {
    Box(
        modifier = Modifier
            .filterTouchesWhenObscured  // Prevents overlay attacks
    ) {
        // Login content
    }
}
```

### 9. Input sanitization

```kotlin
// ✓ GOOD — validate before sending to server
fun validateMessage(content: String): Result<String> {
    val trimmed = content.trim()
    return when {
        trimmed.isEmpty() -> Result.failure(ValidationError("Message cannot be empty"))
        trimmed.length > 4000 -> Result.failure(ValidationError("Message too long (max 4000 chars)"))
        else -> Result.success(trimmed)
    }
}
```

### 10. Session management

```kotlin
// Logout = clear everything
fun logout() {
    secureTokenStore.clearTokens()
    // Clear any in-memory cached user data
    // Navigate to login screen
    // If using Room, clear databases with sensitive data
}