# ECHO Android — Dependencies & Build Configuration

## Version Catalog (`gradle/libs.versions.toml`)

```toml
[versions]
agp = "8.7.0"
kotlin = "2.0.21"
compose-bom = "2024.09.00"
lifecycle = "2.8.6"
hilt = "2.51.1"
hilt-navigation-compose = "1.2.0"
retrofit = "2.11.0"
okhttp = "4.12.0"
kotlinx-serialization = "1.7.3"
kotlinx-serialization-converter = "1.7.3"
kotlinx-coroutines = "1.9.0"
timber = "5.0.1"
coil = "2.7.0"
security-crypto = "1.1.0-alpha06"
junit = "4.13.2"
espresso = "3.6.1"
androidx-junit = "1.2.1"
kotlinx-coroutines-test = "1.9.0"
ksp = "2.0.21-1.0.25"

[libraries]
# Compose BOM (manages all Compose versions)
androidx-compose-bom = { group = "androidx.compose", name = "compose-bom", version.ref = "compose-bom" }

# Compose
androidx-compose-material3 = { group = "androidx.compose.material3", name = "material3" }
androidx-compose-ui = { group = "androidx.compose.ui", name = "ui" }
androidx-compose-ui-graphics = { group = "androidx.compose.ui", name = "ui-graphics" }
androidx-compose-ui-tooling-preview = { group = "androidx.compose.ui", name = "ui-tooling-preview" }
androidx-compose-ui-tooling = { group = "androidx.compose.ui", name = "ui-tooling" }
androidx-compose-ui-test-junit4 = { group = "androidx.compose.ui", name = "ui-test-junit4" }
androidx-compose-ui-test-manifest = { group = "androidx.compose.ui", name = "ui-test-manifest" }

# Core AndroidX
androidx-core-ktx = { group = "androidx.core", name = "core-ktx", version = "1.13.1" }
androidx-activity-compose = { group = "androidx.activity", name = "activity-compose", version = "1.9.2" }
androidx-lifecycle-runtime-ktx = { group = "androidx.lifecycle", name = "lifecycle-runtime-ktx", version.ref = "lifecycle" }
androidx-lifecycle-runtime-compose = { group = "androidx.lifecycle", name = "lifecycle-runtime-compose", version.ref = "lifecycle" }
androidx-lifecycle-viewmodel-compose = { group = "androidx.lifecycle", name = "lifecycle-viewmodel-compose", version.ref = "lifecycle" }

# Navigation
androidx-navigation-compose = { group = "androidx.navigation", name = "navigation-compose", version = "2.8.0" }

# Hilt
hilt-android = { group = "com.google.dagger", name = "hilt-android", version.ref = "hilt" }
hilt-compiler = { group = "com.google.dagger", name = "hilt-android-compiler", version.ref = "hilt" }
hilt-navigation-compose = { group = "androidx.hilt", name = "hilt-navigation-compose", version.ref = "hilt-navigation-compose" }

# Network
retrofit = { group = "com.squareup.retrofit2", name = "retrofit", version.ref = "retrofit" }
retrofit-kotlinx-serialization = { group = "com.squareup.retrofit2", name = "converter-kotlinx-serialization", version.ref = "retrofit" }
okhttp = { group = "com.squareup.okhttp3", name = "okhttp", version.ref = "okhttp" }
okhttp-logging = { group = "com.squareup.okhttp3", name = "logging-interceptor", version.ref = "okhttp" }

# Serialization
kotlinx-serialization-json = { group = "org.jetbrains.kotlinx", name = "kotlinx-serialization-json", version.ref = "kotlinx-serialization" }

# Coroutines
kotlinx-coroutines-android = { group = "org.jetbrains.kotlinx", name = "kotlinx-coroutines-android", version.ref = "kotlinx-coroutines" }
kotlinx-coroutines-core = { group = "org.jetbrains.kotlinx", name = "kotlinx-coroutines-core", version.ref = "kotlinx-coroutines" }

# Utils
timber = { group = "com.jakewharton.timber", name = "timber", version.ref = "timber" }
coil-compose = { group = "io.coil-kt", name = "coil-compose", version.ref = "coil" }

# Security
androidx-security-crypto = { group = "androidx.security", name = "security-crypto", version.ref = "security-crypto" }

# Testing
junit = { group = "junit", name = "junit", version.ref = "junit" }
androidx-junit = { group = "androidx.test.ext", name = "junit", version.ref = "androidx-junit" }
androidx-espresso-core = { group = "androidx.test.espresso", name = "espresso-core", version.ref = "espresso" }
kotlinx-coroutines-test = { group = "org.jetbrains.kotlinx", name = "kotlinx-coroutines-test", version.ref = "kotlinx-coroutines-test" }
hilt-android-testing = { group = "com.google.dagger", name = "hilt-android-testing", version.ref = "hilt" }
hilt-compiler-testing = { group = "com.google.dagger", name = "hilt-android-compiler", version.ref = "hilt" }

[plugins]
android-application = { id = "com.android.application", version.ref = "agp" }
kotlin-compose = { id = "org.jetbrains.kotlin.plugin.compose", version.ref = "kotlin" }
kotlin-serialization = { id = "org.jetbrains.kotlin.plugin.serialization", version.ref = "kotlin" }
hilt = { id = "com.google.dagger.hilt.android", version.ref = "hilt" }
ksp = { id = "com.google.devtools.ksp", version.ref = "ksp" }
```

---

## App-Level `build.gradle.kts`

```kotlin
plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.hilt)
    alias(libs.plugins.ksp)
}

android {
    namespace = "com.shoonya.echo"
    compileSdk = 37

    defaultConfig {
        applicationId = "com.Shoonya.Echo"
        minSdk = 34
        targetSdk = 36
        versionCode = 1
        versionName = "1.0"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"

        // BuildConfig defaults
        buildConfigField("String", "API_BASE_URL", "\"https://echo.example.com\"")
        buildConfigField("String", "WS_BASE_URL", "\"wss://echo.example.com\"")
        buildConfigField("boolean", "ENABLE_LOGGING", "false")
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
        }
        debug {
            isMinifyEnabled = false
            isDebuggable = true
            buildConfigField("String", "API_BASE_URL", "\"http://10.0.2.2:8080\"")
            buildConfigField("String", "WS_BASE_URL", "\"ws://10.0.2.2:8080\"")
            buildConfigField("boolean", "ENABLE_LOGGING", "true")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_11
        targetCompatibility = JavaVersion.VERSION_11
    }

    kotlinOptions {
        jvmTarget = "11"
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    lint {
        disable.add("MissingTranslation")
        informational.add("UnusedResources")
        checkDependencies = true
    }
}

dependencies {
    // Compose BOM (manages Compose library versions)
    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.compose.material3)
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.graphics)
    implementation(libs.androidx.compose.ui.tooling.preview)

    // Core Android
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)

    // Navigation
    implementation(libs.androidx.navigation.compose)

    // Hilt
    implementation(libs.hilt.android)
    ksp(libs.hilt.compiler)
    implementation(libs.hilt.navigation.compose)

    // Network
    implementation(libs.retrofit)
    implementation(libs.retrofit.kotlinx.serialization)
    implementation(libs.okhttp)
    implementation(libs.okhttp.logging)
    implementation(libs.kotlinx.serialization.json)

    // Coroutines
    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.kotlinx.coroutines.core)

    // Utils
    implementation(libs.timber)
    implementation(libs.coil.compose)

    // Security
    implementation(libs.androidx.security.crypto)

    // Testing
    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    androidTestImplementation(platform(libs.androidx.compose.bom))
    androidTestImplementation(libs.androidx.compose.ui.test.junit4)
    androidTestImplementation(libs.androidx.espresso.core)
    androidTestImplementation(libs.androidx.junit)
    androidTestImplementation(libs.hilt.android.testing)
    kspAndroidTest(libs.hilt.compiler.testing)
    debugImplementation(libs.androidx.compose.ui.test.manifest)
    debugImplementation(libs.androidx.compose.ui.tooling)
}
```

---

## ProGuard Rules (`proguard-rules.pro`)

```properties
# kotlinx.serialization
-keepattributes *Annotation*, InnerClasses
-dontnote kotlinx.serialization.AnnotationsKt

-keepclassmembers class kotlinx.serialization.json.** {
    *** Companion;
}
-keepclasseswithmembers class kotlinx.serialization.json.** {
    kotlinx.serialization.KSerializer serializer(...);
}

# ECHO models
-keep,includedescriptorclasses class com.shoonya.echo.**$$serializer { *; }
-keepclassmembers class com.shoonya.echo.** {
    *** Companion;
}
-keepclasseswithmembers class com.shoonya.echo.** {
    kotlinx.serialization.KSerializer serializer(...);
}

# Retrofit
-keepattributes Signature
-keepattributes Exceptions
-keepattributes *Annotation*
-keep class retrofit2.** { *; }
-keepclasseswithmembers class * {
    @retrofit2.http.* <methods>;
}

# OkHttp
-dontwarn okhttp3.**
-dontwarn okio.**

# Hilt
-keep class dagger.hilt.** { *; }
-keep class javax.inject.** { *; }
-keep class * extends dagger.hilt.android.internal.managers.ViewComponentManager$FragmentContextWrapper { *; }
```

---

## Android Manifest (`AndroidManifest.xml`)

```xml
<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    xmlns:tools="http://schemas.android.com/tools">

    <uses-permission android:name="android.permission.INTERNET" />
    <uses-permission android:name="android.permission.ACCESS_NETWORK_STATE" />

    <application
        android:name=".EchoApplication"
        android:allowBackup="true"
        android:icon="@mipmap/ic_launcher"
        android:label="ECHO"
        android:roundIcon="@mipmap/ic_launcher_round"
        android:supportsRtl="true"
        android:theme="@style/Theme.ECHO"
        android:usesCleartextTraffic="false"
        android:networkSecurityConfig="@xml/network_security_config"
        tools:targetApi="34">

        <activity
            android:name=".MainActivity"
            android:exported="true"
            android:theme="@style/Theme.ECHO"
            android:windowSoftInputMode="adjustResize">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
    </application>
</manifest>
```

---

## Network Security Config (`res/xml/network_security_config.xml`)

```xml
<?xml version="1.0" encoding="utf-8"?>
<network-security-config>
    <!-- Production: HTTPS only -->
    <domain-config cleartextTrafficPermitted="false">
        <domain includeSubdomains="true">echo.example.com</domain>
        <trust-anchors>
            <certificates src="system" />
        </trust-anchors>
    </domain-config>

    <!-- Debug: allow localhost for emulator -->
    <domain-config cleartextTrafficPermitted="true">
        <domain includeSubdomains="false">10.0.2.2</domain>
        <domain includeSubdomains="false">localhost</domain>
    </domain-config>
</network-security-config>
```

---

## Dependency Purpose Summary

| Dependency | Purpose |
|---|---|
| `androidx.compose.bom` | Manages all Compose library versions |
| `material3` | Material Design 3 components |
| `navigation-compose` | Type-safe navigation between screens |
| `hilt-android` | Dependency injection framework |
| `retrofit` | Type-safe HTTP client for REST APIs |
| `okhttp` | HTTP/WebSocket client |
| `kotlinx-serialization-json` | Compile-time JSON serialization |
| `kotlinx-coroutines-android` | Async programming + `Dispatchers.Main` |
| `lifecycle-runtime-compose` | `collectAsStateWithLifecycle()` |
| `lifecycle-viewmodel-compose` | `viewModel()` composable function |
| `coil-compose` | Async image loading (`AsyncImage`) |
| `security-crypto` | `EncryptedSharedPreferences` for JWT |
| `timber` | Structured logging |

## Key Gradle Plugins

| Plugin | Purpose |
|---|---|
| `com.android.application` | Android build system |
| `kotlin.plugin.compose` | Compose compiler (K2) |
| `kotlin.plugin.serialization` | `@Serializable` annotation processing |
| `dagger.hilt.android` | Hilt annotation processing |
| `com.google.devtools.ksp` | Kotlin Symbol Processing (Hilt compiler) |

## Notes

- **No Gson, no Moshi** — kotlinx.serialization is the only JSON library per rules.
- **Compose versions are never pinned** — only via BOM.
- **Two OkHttpClient instances**: one for REST (with auth interceptor + logging), one for WebSocket (infinite read timeout, ping interval).
- **KSP is used** (not kapt) for Hilt compilation — faster, Kotlin-native.