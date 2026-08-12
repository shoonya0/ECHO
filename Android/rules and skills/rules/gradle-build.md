# Gradle Build — ECHO Android

## Rule

1. **Version catalog** (`libs.versions.toml`) for all dependency versions — no version strings in `build.gradle.kts`.
2. **Target SDK 36, min SDK 34** (matching the existing `androidbuild.gradle.kts`). Compile SDK 37.
3. **Compose BOM** manages all Compose dependency versions — individual Compose library versions must not be pinned.
4. **Kotlin 2.x** with Compose compiler plugin integrated via the Kotlin compose plugin (not a standalone compiler extension).
5. **Build types:** `debug` and `release`. Release enables R8 minification, proguard rules, and disables logging.

## Why

- The version catalog prevents dependency version drift and makes upgrades a single-file change.
- Compose BOM guarantees that all Compose libraries are compatible with each other — no mismatched versions.
- Target SDK 36 and min SDK 34 align with the Go backend's compileSdk 37 configuration, targeting modern Android.

## How to apply

### 1. Version catalog (`gradle/libs.versions.toml`)

```toml
[versions]
agp = "8.7.0"
kotlin = "2.0.20"
compose-bom = "2024.08.00"
lifecycle = "2.8.6"
hilt = "2.51.1"
retrofit = "2.11.0"
okhttp = "4.12.0"
kotlinx-serialization = "1.7.1"
kotlinx-coroutines = "1.8.1"
timber = "5.0.1"
coil = "2.7.0"
junit = "4.13.2"
espresso = "3.6.1"

[libraries]
# Compose BOM
androidx-compose-bom = { group = "androidx.compose", name = "compose-bom", version.ref = "compose-bom" }
# Compose
androidx-compose-material3 = { group = "androidx.compose.material3", name = "material3" }
androidx-compose-ui = { group = "androidx.compose.ui", name = "ui" }
androidx-compose-ui-graphics = { group = "androidx.compose.ui", name = "ui-graphics" }
androidx-compose-ui-tooling-preview = { group = "androidx.compose.ui", name = "ui-tooling-preview" }
androidx-compose-ui-tooling = { group = "androidx.compose.ui", name = "ui-tooling" }
androidx-compose-ui-test-junit4 = { group = "androidx.compose.ui", name = "ui-test-junit4" }
androidx-compose-ui-test-manifest = { group = "androidx.compose.ui", name = "ui-test-manifest" }
# Core
androidx-core-ktx = { group = "androidx.core", name = "core-ktx", version = "1.13.1" }
androidx-activity-compose = { group = "androidx.activity", name = "activity-compose", version = "1.9.1" }
androidx-lifecycle-runtime-ktx = { group = "androidx.lifecycle", name = "lifecycle-runtime-ktx", version.ref = "lifecycle" }
androidx-lifecycle-runtime-compose = { group = "androidx.lifecycle", name = "lifecycle-runtime-compose", version.ref = "lifecycle" }
# Hilt
hilt-android = { group = "com.google.dagger", name = "hilt-android", version.ref = "hilt" }
hilt-compiler = { group = "com.google.dagger", name = "hilt-android-compiler", version.ref = "hilt" }
hilt-navigation-compose = { group = "androidx.hilt", name = "hilt-navigation-compose", version = "1.2.0" }
# Network
retrofit = { group = "com.squareup.retrofit2", name = "retrofit", version.ref = "retrofit" }
retrofit-kotlinx-serialization = { group = "com.squareup.retrofit2", name = "converter-kotlinx-serialization", version.ref = "retrofit" }
okhttp = { group = "com.squareup.okhttp3", name = "okhttp", version.ref = "okhttp" }
okhttp-logging = { group = "com.squareup.okhttp3", name = "logging-interceptor", version.ref = "okhttp" }
kotlinx-serialization-json = { group = "org.jetbrains.kotlinx", name = "kotlinx-serialization-json", version.ref = "kotlinx-serialization" }
kotlinx-coroutines-android = { group = "org.jetbrains.kotlinx", name = "kotlinx-coroutines-android", version.ref = "kotlinx-coroutines" }
# Utils
timber = { group = "com.jakewharton.timber", name = "timber", version.ref = "timber" }
coil-compose = { group = "io.coil-kt", name = "coil-compose", version.ref = "coil" }
# Security
androidx-security-crypto = { group = "androidx.security", name = "security-crypto", version = "1.1.0-alpha06" }
# Testing
junit = { group = "junit", name = "junit", version.ref = "junit" }
androidx-junit = { group = "androidx.test.ext", name = "junit", version = "1.2.1" }
androidx-espresso-core = { group = "androidx.test.espresso", name = "espresso-core", version.ref = "espresso" }
kotlinx-coroutines-test = { group = "org.jetbrains.kotlinx", name = "kotlinx-coroutines-test", version.ref = "kotlinx-coroutines" }
hilt-android-testing = { group = "com.google.dagger", name = "hilt-android-testing", version.ref = "hilt" }
hilt-compiler-testing = { group = "com.google.dagger", name = "hilt-android-compiler", version.ref = "hilt" }

[plugins]
android-application = { id = "com.android.application", version.ref = "agp" }
kotlin-compose = { id = "org.jetbrains.kotlin.plugin.compose", version.ref = "kotlin" }
kotlin-serialization = { id = "org.jetbrains.kotlin.plugin.serialization", version.ref = "kotlin" }
hilt = { id = "com.google.dagger.hilt.android", version.ref = "hilt" }
ksp = { id = "com.google.devtools.ksp", version = "2.0.20-1.0.24" }
```

### 2. Build optimization

```kotlin
// androidbuild.gradle.kts additions
android {
    // ... existing config

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
            // Allow cleartext for emulator
            buildConfigField("String", "API_BASE_URL", "\"http://10.0.2.2:8080\"")
            buildConfigField("String", "WS_BASE_URL", "\"ws://10.0.2.2:8080\"")
        }
    }

    kotlinOptions {
        jvmTarget = "11"
    }
}
```

### 3. Never pin Compose library versions individually

```kotlin
// ✓ GOOD — BOM manages versions
implementation(platform(libs.androidx.compose.bom))
implementation(libs.androidx.compose.material3)
implementation(libs.androidx.compose.ui)

// ✗ BAD — pinned version bypasses BOM, causes crashes
implementation("androidx.compose.material3:material3:1.3.0")
```

### 4. Dependency hygiene

```kotlin
// ✓ Use api() only for transitive dependencies callers need
api(libs.kotlinx.coroutines.android)

// Use implementation() for internal dependencies
implementation(libs.okhttp)
implementation(libs.timber)

// Test dependencies
testImplementation(libs.junit)
testImplementation(libs.kotlinx.coroutines.test)
androidTestImplementation(platform(libs.androidx.compose.bom))
androidTestImplementation(libs.androidx.compose.ui.test.junit4)
androidTestImplementation(libs.hilt.android.testing)
kspAndroidTest(libs.hilt.compiler.testing)
debugImplementation(libs.androidx.compose.ui.tooling)
debugImplementation(libs.androidx.compose.ui.test.manifest)
```

### 5. Lint and code quality

```kotlin
android {
    lint {
        disable.add("MissingTranslation")  // We don't localize yet
        informational.add("UnusedResources")
        checkDependencies = true
    }
}
```

### 6. BuildConfig fields

```kotlin
defaultConfig {
    // Always define these — values overridden per build type
    buildConfigField("String", "API_BASE_URL", "\"https://echo.example.com\"")
    buildConfigField("String", "WS_BASE_URL", "\"wss://echo.example.com\"")
    buildConfigField("boolean", "ENABLE_LOGGING", "false")
}