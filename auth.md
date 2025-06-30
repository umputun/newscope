# Multi-User Authentication Plan (Using go-pkgz/auth)

## Overview

Add optional multi-user support with three roles: READ (view only), WRITE (vote/feedback), and ADMIN (user management). Leverage `github.com/go-pkgz/auth` library for secure authentication with email/password, GitHub OAuth, and Telegram.

## Core Requirements

1. **Optional auth** - controlled by `auth.enabled` config flag
2. **Public read mode** - when `auth.public_read=true`, unauthenticated users can read articles
3. **Three roles**:
   - **READ**: View articles only
   - **WRITE**: Vote, provide feedback, manage preferences  
   - **ADMIN**: Manage users (including role changes and deletion)
4. **Admin-created users only** - Admin chooses auth method: email/password OR GitHub
5. **Backwards compatible** - works without auth when disabled
6. **No blocking mechanism** - Access control via role changes or user deletion

## Implementation

### 1. Database Changes

```sql
-- Simple users table for admin-managed users
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT,  -- NULL for OAuth users
    role TEXT NOT NULL CHECK (role IN ('read', 'write', 'admin')),
    provider TEXT NOT NULL CHECK (provider IN ('email', 'github', 'telegram')),
    github_id TEXT UNIQUE,  -- GitHub numeric user ID (immutable)
    github_username TEXT,  -- GitHub username (for display only)
    telegram_id TEXT UNIQUE,  -- Telegram user ID
    telegram_username TEXT,  -- Telegram username (optional, for display)
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_login_at TIMESTAMP
);

-- Create indexes
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_github_id ON users(github_id);
CREATE INDEX idx_users_telegram_id ON users(telegram_id);
```

Note: go-pkgz/auth handles sessions via JWT tokens, so no sessions table needed.

### 2. Configuration

```yaml
auth:
  enabled: false          # Enable authentication
  public_read: true       # Allow unauthenticated reading
  jwt_secret: ${JWT_SECRET}  # Secret for JWT signing
  cookie_duration: 168h   # Cookie/token duration (1 week)
  
  # Admin user (created on startup if missing)
  admin:
    email: ${ADMIN_EMAIL}
    password: ${ADMIN_PASSWORD}
  
  # OAuth providers (for pre-authorized users only)
  github:
    enabled: true
    client_id: ${GITHUB_CLIENT_ID}
    client_secret: ${GITHUB_CLIENT_SECRET}
    
  telegram:
    enabled: true
    token: ${TELEGRAM_TOKEN}  # Bot token from @BotFather
```

### 3. Auth Service Setup

```go
// Initialize auth service
func (s *Server) setupAuth() (*auth.Service, error) {
    authOpts := auth.Opts{
        SecretReader:   auth.SecretFunc(func() (string, error) { return s.config.Auth.JWTSecret, nil }),
        TokenDuration:  s.config.Auth.CookieDuration,
        CookieDuration: s.config.Auth.CookieDuration,
        Issuer:         "newscope",
        URL:            s.config.Server.BaseURL,
        Validator: auth.ValidatorFunc(s.validateUser),
        ClaimsUpd: auth.ClaimsUpdFunc(s.updateClaims),
    }
    
    service := auth.NewService(authOpts)
    
    // Add direct provider for email/password
    service.AddDirectProvider("email", provider.CredCheckerFunc(s.checkCredentials))
    
    // Add GitHub provider if enabled
    if s.config.Auth.GitHub.Enabled {
        service.AddProvider("github", s.config.Auth.GitHub.ClientID, s.config.Auth.GitHub.ClientSecret)
    }
    
    // Add Telegram provider if enabled
    if s.config.Auth.Telegram.Enabled {
        telegram := &provider.TelegramHandler{
            ProviderName: "telegram",
            SuccessMsg:   "✅ You have successfully authenticated, check the web!",
            Telegram:     provider.NewTelegramAPI(s.config.Auth.Telegram.Token, &http.Client{Timeout: 5 * time.Second}),
            L:            log.Default(),
            TokenService: service.TokenService(),
            AvatarSaver:  service.AvatarProxy(),
        }
        service.AddCustomHandler(telegram)
    }
    
    return service, nil
}

// Validate credentials against database
func (s *Server) checkCredentials(user, password string) (bool, error) {
    // Pre-generated bcrypt hash for timing attack mitigation
    dummyHash := "$2a$10$dummyHashForTimingAttackMitigation....................."
    
    u, err := s.repos.Users.GetByEmail(user)
    if err != nil {
        // Always perform hash comparison to prevent timing attacks
        _ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
        return false, nil // Don't reveal user existence
    }
    
    err = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
    return err == nil, nil
}

// Add role to claims from database
func (s *Server) updateClaims(claims auth.Claims) auth.Claims {
    var user *domain.User
    
    switch {
    case claims.User.ID != "" && strings.HasPrefix(claims.User.ID, "github_"):
        // GitHub user - check if pre-authorized by immutable ID
        githubID := strings.TrimPrefix(claims.User.ID, "github_")
        user, err = s.repos.Users.GetByGitHubID(githubID)
        if err != nil {
            log.Printf("[WARN] failed to get user by GitHub ID %s: %v", githubID, err)
        }
        
    case claims.User.ID != "" && strings.HasPrefix(claims.User.ID, "telegram_"):
        // Telegram user - check if pre-authorized
        telegramID := strings.TrimPrefix(claims.User.ID, "telegram_")
        user, err = s.repos.Users.GetByTelegramID(telegramID)
        if err != nil {
            log.Printf("[WARN] failed to get user by Telegram ID %s: %v", telegramID, err)
        }
        
    default:
        // Email auth user
        user, err = s.repos.Users.GetByEmail(claims.User.Email)
        if err != nil {
            log.Printf("[WARN] failed to get user by email %s: %v", claims.User.Email, err)
        }
    }
    
    if user != nil {
        claims.User.SetStrAttr("role", user.Role)
        claims.User.SetStrAttr("user_id", fmt.Sprintf("%d", user.ID))
        
        // Set admin flag for go-pkgz/auth AdminOnly middleware compatibility
        if user.Role == "admin" {
            claims.User.SetAdmin(true)
        }
        
        // Update last login
        s.repos.Users.UpdateLastLogin(user.ID)
        
        // For GitHub users, update the GitHub ID if not set (first login)
        if user.Provider == "github" && user.GitHubID == "" && strings.HasPrefix(claims.User.ID, "github_") {
            githubID := strings.TrimPrefix(claims.User.ID, "github_")
            if err := s.repos.Users.UpdateGitHubID(user.ID, githubID); err != nil {
                log.Printf("[WARN] failed to update GitHub ID for user %d: %v", user.ID, err)
            }
        }
    }
    
    return claims
}

// Validate that user exists in our database
func (s *Server) validateUser(token string, claims auth.Claims) bool {
    // Check if user exists in our database by checking if role was set
    userRole := claims.User.StrAttr("role")
    return userRole != "" // User was found and has a role
}
```

### 4. Middleware Implementation

```go
// Role-based authorization middleware
func (s *Server) requireRole(role string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Skip if auth disabled
            if !s.config.Auth.Enabled {
                next.ServeHTTP(w, r)
                return
            }
            
            // Allow public read if configured
            if role == "" && s.config.Auth.PublicRead {
                next.ServeHTTP(w, r)
                return
            }
            
            // Get user from go-pkgz/auth middleware
            user, err := token.GetUserInfo(r)
            if err != nil {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }
            
            // Check role hierarchy
            userRole := user.StrAttr("role")
            if !hasRole(userRole, role) {
                http.Error(w, "Forbidden", http.StatusForbidden)
                return
            }
            
            next.ServeHTTP(w, r)
        })
    }
}

// Role hierarchy: admin > write > read
func hasRole(userRole, requiredRole string) bool {
    if requiredRole == "" {
        return true
    }
    roles := map[string]int{"read": 1, "write": 2, "admin": 3}
    return roles[userRole] >= roles[requiredRole]
}
```

### 5. Route Setup

```go
// Setup auth service
authService, _ := s.setupAuth()

// Auth routes - go-pkgz/auth handles all the endpoints automatically
authHandler := authService.Handlers()
router.Mount("/auth", authHandler)

// Apply auth middleware to all routes
router.Use(authService.Middleware())

// Public routes (conditional auth)
router.Mount("/api").Route(func(r *routegroup.Bundle) {
    r.With(s.requireRole("")).GET("/articles", s.getArticles)
    r.With(s.requireRole("")).GET("/feeds", s.getFeeds)
})

// Write routes
router.Mount("/api").Route(func(r *routegroup.Bundle) {
    r.With(s.requireRole("write")).POST("/vote/{id}", s.vote)
    r.With(s.requireRole("write")).DELETE("/vote/{id}", s.unvote)
    r.With(s.requireRole("write")).GET("/preferences", s.getPreferences)
    r.With(s.requireRole("write")).PUT("/preferences", s.updatePreferences)
})

// Admin routes
router.Mount("/api/admin").With(s.requireRole("admin")).Route(func(r *routegroup.Bundle) {
    r.GET("/users", s.listUsers)
    r.POST("/users", s.createUser)
    r.PUT("/users/{id}", s.updateUser)
    r.DELETE("/users/{id}", s.deleteUser)
})
```

### 6. User Management

#### Admin Creates Users (Email or GitHub)

```go
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Email            string `json:"email"`
        Password         string `json:"password"`         // Only for email users
        GitHubUsername   string `json:"github_username"`   // For GitHub users
        TelegramUsername string `json:"telegram_username"` // For Telegram users (optional)
        TelegramID       string `json:"telegram_id"`       // For Telegram users
        Role             string `json:"role"`
        Provider         string `json:"provider"` // "email", "github", or "telegram"
    }
    
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid request", http.StatusBadRequest)
        return
    }
    
    user := &domain.User{
        Email:    req.Email,
        Role:     req.Role,
        Provider: req.Provider,
    }
    
    switch req.Provider {
    case "email":
        // Hash password for email users
        hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
        user.PasswordHash = string(hash)
        
    case "github":
        // Store GitHub username for display
        user.GitHubUsername = req.GitHubUsername
        // Note: GitHub ID will be resolved and stored on first login via claims.User.ID
        
    case "telegram":
        // Store Telegram info for matching during login
        user.TelegramUsername = req.TelegramUsername
        user.TelegramID = req.TelegramID
    }
    
    created, err := s.repos.Users.Create(user)
    if err != nil {
        http.Error(w, "Failed to create user", http.StatusInternalServerError)
        return
    }
    
    s.render(w, "admin/user-row.html", created)
}
```

#### OAuth User Login Flow

When a pre-authorized OAuth user logs in:

**GitHub:**
1. Redirected to GitHub OAuth
2. On callback, go-pkgz/auth creates JWT with GitHub info including numeric ID
3. Our `updateClaims` function checks if GitHub ID exists in database
4. If not found but username matches, store the GitHub ID for future logins
5. If not found at all, `validateUser` returns false, rejecting the login
6. If found, user gets their assigned role

**Telegram:**
1. User clicks Telegram login button, opens Telegram bot
2. User sends `/start` or clicks auth link in bot
3. go-pkgz/auth validates and creates JWT with Telegram info
4. Our `updateClaims` function checks if Telegram ID exists in database
5. If not found, `validateUser` returns false, rejecting the login
6. If found, user gets their assigned role

### 7. UI Implementation

#### Login Page

```html
<div class="login-container">
    <h2>Login to Newscope</h2>
    
    <!-- Email/Password login -->
    <form method="POST" action="/auth/login">
        <input type="email" name="user" placeholder="Email" required>
        <input type="password" name="passwd" placeholder="Password" required>
        <button type="submit">Login</button>
    </form>
    
    <div class="divider">OR</div>
    
    <!-- OAuth logins -->
    <div class="oauth-buttons">
        <a href="/auth/github/login" class="github-login">
            <i class="fab fa-github"></i> Login with GitHub
        </a>
        
        <a href="/auth/telegram/login" class="telegram-login">
            <i class="fab fa-telegram"></i> Login with Telegram
        </a>
    </div>
</div>
```

#### User Info in Header

```html
<div class="user-menu" hx-get="/auth/user" hx-trigger="load">
    <!-- Populated by HTMX -->
</div>

<!-- Response template -->
{{if .user}}
<div class="dropdown">
    <span>{{.user.name}} ({{.user.role}})</span>
    <a href="/auth/logout">Logout</a>
</div>
{{else}}
<a href="/login">Login</a>
{{end}}
```

#### Admin User Management

```html
<div class="admin-users">
    <h2>User Management</h2>
    
    <!-- Add user form -->
    <form hx-post="/api/admin/users" hx-target="#users-table tbody" hx-swap="beforeend">
        <h3>Add User</h3>
        <input type="email" name="email" placeholder="user@example.com" required>
        
        <select name="provider" hx-on:change="toggleAuthFields(this)">
            <option value="email">Email/Password</option>
            <option value="github">GitHub</option>
            <option value="telegram">Telegram</option>
        </select>
        
        <div id="email-fields">
            <input type="password" name="password" placeholder="temporary password">
        </div>
        
        <div id="github-fields" style="display:none">
            <input type="text" name="github_username" placeholder="GitHub username">
        </div>
        
        <div id="telegram-fields" style="display:none">
            <input type="text" name="telegram_username" placeholder="Telegram username (optional)">
            <input type="text" name="telegram_id" placeholder="Telegram ID" required>
            <small>User can get their ID from @userinfobot on Telegram</small>
        </div>
        
        <select name="role">
            <option value="read">Read Only</option>
            <option value="write" selected>Write</option>
            <option value="admin">Admin</option>
        </select>
        <button type="submit">Create User</button>
    </form>
    
    <script>
    function toggleAuthFields(select) {
        document.getElementById('email-fields').style.display = 
            select.value === 'email' ? 'block' : 'none';
        document.getElementById('github-fields').style.display = 
            select.value === 'github' ? 'block' : 'none';
        document.getElementById('telegram-fields').style.display = 
            select.value === 'telegram' ? 'block' : 'none';
    }
    </script>
    
    <table id="users-table">
        <thead>
            <tr>
                <th>Email</th>
                <th>Provider</th>
                <th>Role</th>
                <th>Created</th>
                <th>Actions</th>
            </tr>
        </thead>
        <tbody>
            {{range .Users}}
            <tr>
                <td>{{.Email}}</td>
                <td>
                    {{if eq .Provider "email"}}
                        <i class="fas fa-envelope"></i> Email
                    {{else if eq .Provider "github"}}
                        <i class="fab fa-github"></i> GitHub (@{{.GitHubUsername}})
                    {{else if eq .Provider "telegram"}}
                        <i class="fab fa-telegram"></i> Telegram ({{if .TelegramUsername}}@{{.TelegramUsername}}{{else}}ID: {{.TelegramID}}{{end}})
                    {{end}}
                </td>
                <td>
                    <select hx-put="/api/admin/users/{{.ID}}" 
                            hx-vals='{\"role\": this.value}'
                            hx-trigger="change">
                        <option value="read" {{if eq .Role "read"}}selected{{end}}>Read</option>
                        <option value="write" {{if eq .Role "write"}}selected{{end}}>Write</option>
                        <option value="admin" {{if eq .Role "admin"}}selected{{end}}>Admin</option>
                    </select>
                </td>
                <td>{{.CreatedAt.Format "2006-01-02"}}</td>
                <td>
                    {{if eq .Provider "email"}}
                    <button hx-get="/api/admin/users/{{.ID}}/reset-password">Reset Password</button>
                    {{end}}
                    <button hx-delete="/api/admin/users/{{.ID}}" 
                            hx-confirm="Delete user {{.Email}}?"
                            hx-target="closest tr"
                            hx-swap="outerHTML">Delete</button>
                </td>
            </tr>
            {{end}}
        </tbody>
    </table>
</div>
```

### 8. Migration Strategy

When auth is first enabled:
1. Create admin user from config
2. All existing data remains shared (no user_id needed)
3. Both email/password and GitHub users can coexist

```go
// On startup when auth.enabled = true
if s.config.Auth.Enabled {
    // Create admin if not exists
    admin, _ := s.repos.Users.GetByEmail(s.config.Auth.Admin.Email)
    if admin == nil {
        hash, _ := bcrypt.GenerateFromPassword([]byte(s.config.Auth.Admin.Password), bcrypt.DefaultCost)
        s.repos.Users.Create(&domain.User{
            Email:        s.config.Auth.Admin.Email,
            PasswordHash: string(hash),
            Role:         "admin",
            Provider:     "email",
        })
    }
}
```

### 9. Security Notes

go-pkgz/auth handles:
- JWT token generation and validation
- Secure HTTP-only cookies with XSRF protection
- OAuth state validation
- Session management
- Timing-safe comparisons
- Proper error responses

We only need to:
- Store users and roles in database
- Implement role checking middleware
- Handle user management UI

Important security considerations:
- **JWT Secret**: Must be a high-entropy, cryptographically random string
- **Password Reset**: Admin-created passwords should be temporary, require change on first login
- **GitHub ID**: We store GitHub numeric ID (immutable) not just username (mutable)

### 10. Access Control

Since all users are admin-created:
- **No user blocking needed** - Admin can change role or delete user
- **Role changes are immediate** - Next request will use updated role
- **Deleted users lose access** - JWT validation fails for non-existent users

## Implementation Steps

### Phase 1: Core Infrastructure
- Add go-pkgz/auth dependency
- Database schema and user repository
- User domain model
- Basic auth service setup

### Phase 2: Authentication
- Email/password login with credential checker
- GitHub OAuth integration with username validation
- Claims enrichment with roles
- Session management

### Phase 3: Authorization
- Role-based middleware
- Update all routes with appropriate role requirements
- Public read mode handling
- Context propagation

### Phase 4: UI Integration
- Login page with auth method selection
- User info in header
- Conditional UI elements based on roles
- HTMX integration for auth flows

### Phase 5: Admin Features
- User management page
- Create users with email or GitHub auth
- Role editing
- Password reset for email users

### Phase 6: Testing & Polish
- Unit tests for auth components
- Integration tests for auth flows
- Security review
- Documentation

## Benefits of This Approach

- **Proven security** - go-pkgz/auth is battle-tested
- **Less code** - No need to implement JWT, sessions, CSRF
- **Flexible** - Supports both managed users and social login
- **Future-proof** - Easy to add more OAuth providers
- **Simple data model** - All data remains shared, auth only controls access