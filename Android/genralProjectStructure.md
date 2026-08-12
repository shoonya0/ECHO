com.yourpackage.app/
│
├── core/                         # Global, shared, feature-agnostic code
│   ├── data/                     # Global networking, local DB configs, shared
									preferences
│   │   ├── local/
│   │   └── remote/
│   ├── di/                       # Global Dependency Injection modules (Hilt / Koin)
│   ├── theme/                    # Jetpack Compose Design System (Color, Type, Theme)
│   └── util/                     # Extension functions and universal utilities
│
└── features/                     # Business modules organized by feature
    ├── auth/                     # Example: Authentication Feature
    │   ├── data/                 # Data Layer: API services, local entities, repositories
    │   │   ├── model/            # DTOs and database entities
    │   │   ├── remote/           # AuthApiInterface
    │   │   └── repository/       # AuthRepositoryImpl
    │   │
    │   ├── domain/               # Domain Layer: Business rules & plain Kotlin models
    │   │   ├── model/            # Clean domain data classes (e.g., User)
    │   │   ├── repository/       # AuthRepository interface
    │   │   └── usecase/          # LoginUseCase, RegisterUseCase
    │   │
    │   └── presentation/         # UI Layer: Jetpack Compose & State management
    │       ├── LoginScreen.kt    # Composable UI
    │       ├── LoginState.kt     # UI State state holder
    │       └── LoginViewModel.kt # ViewModel
    │
    └── home/                     # Example: Home Feature (repeats the same structural layers)
        ├── data/
        ├── domain/
        └── presentation/


Inner Structure

features/
└── payment/
    ├── data/
    │   └── model/
    │       ├── PaymentRequest.kt       # Data Class (API Payload)
    │       └── PaymentResponse.kt      # Data Class (API JSON Response)
    │
    ├── domain/
    │   ├── model/
    │   │   ├── Transaction.kt          # Data Class (Clean Domain Model)
    │   │   ├── TransactionStatus.kt    # Enum (PENDING, SUCCESS, FAILED)
    │   │   └── PaymentResult.kt        # Sealed Class (Success(T) / Error)
    │   └── usecase/
    │       └── ProcessPaymentUseCase.kt
    │
    └── presentation/
        ├── PaymentScreen.kt            # Composable Function
        ├── PaymentUiState.kt           # Data Class (Holds UI state fields)
        ├── PaymentUiEvent.kt           # Sealed Interface (User actions: ClickSubmit, ClickCancel)
        └── PaymentViewModel.kt         # Class
