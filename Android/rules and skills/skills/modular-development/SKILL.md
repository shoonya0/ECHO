---
name: modular-development
description: "Write modular, feature-first Kotlin code following ECHO Android's clean architecture. Triggers on 'add feature', 'new screen', 'modular', 'feature boundary', or 'clean architecture'."
user-invocable: true
metadata:
  version: "1.0.0"
allowed-tools: Read Edit Write Bash Globbing Grep
---

**Persona:** You are an Android architect who enforces feature-first clean architecture. Every feature is independent; core is shared infrastructure only. Cross-feature imports are a design smell.

> **Project rules:** Read `Android/rules and skills/rules/clean-architecture.md` and `Android/rules and skills/rules/modularity.md` before using this skill.

# Modular Development Checklist

## Starting a New Feature

Follow these steps when adding a new feature to the ECHO Android app:

### Step 1: Create the 3-file contract
- [ ] Domain model: `features/{feature}/domain/model/{Feature}Item.kt`
- [ ] Repository interface: `features/{feature}/domain/repository/{Feature}Repository.kt`
- [ ] UI state: `features/{feature}/presentation/{Feature}UiState.kt`

```kotlin
// 1. Domain model
data class ContactItem(
    val id: String,
    val username: String,
    val displayName: String,
    val avatarUrl: String?,
    val isOnline: Boolean,
)

// 2. Repository interface
interface ContactRepository {
    suspend fun getContacts(): Result<List<ContactItem>>
    suspend fun searchContacts(query: String): Result<List<ContactItem>>
}

// 3. UI State
data class ContactsUiState(
    val isLoading: Boolean = false,
    val contacts: List<ContactItem> = emptyList(),
    val searchQuery: String = "",
    val error: String? = null,
)
```

### Step 2: Implement the data layer
- [ ] API service interface: `features/{feature}/data/remote/{Feature}ApiService.kt`
- [ ] DTO models with `@Serializable`: `features/{feature}/data/model/`
- [ ] Repository implementation: `features/{feature}/data/repository/{Feature}RepositoryImpl.kt`
- [ ] DTO-to-domain mapper extension: `ContactResponse.toDomain(): ContactItem`
- [ ] DI module: `features/{feature}/data/di/{Feature}Module.kt`

### Step 3: Implement the domain layer
- [ ] Use cases: `features/{feature}/domain/usecase/`
- [ ] One use case = one public method = one file
- [ ] Use cases take repository via `@Inject constructor`

```kotlin
class GetContactsUseCase @Inject constructor(
    private val repo: ContactRepository
) {
    suspend operator fun invoke(): Result<List<ContactItem>> = repo.getContacts()
}
```

### Step 4: Implement the presentation layer
- [ ] Events: `features/{feature}/presentation/{Feature}Event.kt` (sealed interface)
- [ ] ViewModel: `features/{feature}/presentation/{Feature}ViewModel.kt`
- [ ] Screen composable: `features/{feature}/presentation/{Feature}Screen.kt`
- [ ] Content composable (stateless): in same file or separate

### Step 5: Register the screen
- [ ] Add route to `app/MainNavigation.kt`
- [ ] Add composable destination in `NavHost`

## Feature Boundaries — Dos and Don'ts

| Do | Don't |
|---|---|
| Import from `core/util/`, `core/theme/`, `core/data/` | Import from `features/otherfeature/` |
| Pass primitive IDs via navigation arguments | Share ViewModels between features |
| Create a shared interface in `core/` if two features need the same contract | Have feature A's repository call feature B's API service |
| Use `core/util/ErrorMapper.toUserMessage()` | Write feature-specific error mapping |

## Adding a New Dependency Injection Module

```kotlin
// features/{feature}/data/di/{Feature}Module.kt
@Module
@InstallIn(SingletonComponent::class)
abstract class ContactsModule {
    @Binds @Singleton
    abstract fun bindContactsRepository(impl: ContactsRepositoryImpl): ContactRepository
}

@Module
@InstallIn(SingletonComponent::class)
object ContactsNetworkModule {
    @Provides @Singleton
    fun provideContactsApi(retrofit: Retrofit): ContactsApiService =
        retrofit.create(ContactsApiService::class.java)
}
```

## Verify Modularity

After implementation, check:
- [ ] No feature package imports another feature's internals
- [ ] Core package has zero feature imports
- [ ] `./gradlew assembleDebug` passes
- [ ] All new files follow the directory structure from `clean-architecture.md`

## Cross-References

- → `Android/rules and skills/rules/clean-architecture.md` — Layer responsibilities
- → `Android/rules and skills/rules/modularity.md` — Feature boundaries
- → `Android/rules and skills/rules/dependency-injection.md` — Hilt module setup