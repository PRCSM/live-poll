package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/crypto/bcrypt"

	"github.com/PRCSM/live-poll/backend/internal/apperr"
	"github.com/PRCSM/live-poll/backend/internal/httpx"
	"github.com/PRCSM/live-poll/backend/internal/ratelimit"
)

const (
	bcryptCost = 12
	userKey    = "auth_user"
)

var errBadCredentials = apperr.New(http.StatusUnauthorized, "invalid_credentials", "Email or password is incorrect.")

type Handler struct {
	users   *Users
	tokens  *Tokens
	limiter *ratelimit.Limiter
	// dummyHash is compared against when the email does not exist, so a login for an
	// unknown email takes as long as one with a wrong password.
	dummyHash []byte
}

func NewHandler(users *Users, tokens *Tokens, limiter *ratelimit.Limiter) (*Handler, error) {
	dummy, err := bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcryptCost)
	if err != nil {
		return nil, err
	}
	return &Handler{users: users, tokens: tokens, limiter: limiter, dummyHash: dummy}, nil
}

func (h *Handler) Register(api *gin.RouterGroup) {
	g := api.Group("/auth")
	ipLimit := h.limiter.Middleware("auth-ip", 5, time.Minute, ratelimit.ByIP)
	g.POST("/signup", ipLimit, h.signup)
	g.POST("/login", ipLimit, h.login)
	g.POST("/logout", h.Required(), h.logout)
	g.GET("/me", h.Required(), h.me)
}

type session struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
	User      *User     `json:"user"`
}

func (h *Handler) signup(c *gin.Context) {
	var in signupInput
	if err := httpx.DecodeJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	if fields := in.validate(); len(fields) > 0 {
		httpx.Fail(c, apperr.Validation(fields))
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcryptCost)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	u := &User{ID: bson.NewObjectID(), Email: in.Email, Name: in.Name, PasswordHash: string(hash), CreatedAt: time.Now().UTC()}
	if err := h.users.Create(c.Request.Context(), u); err != nil {
		if errors.Is(err, errEmailTaken) {
			httpx.Fail(c, apperr.Validation(map[string]string{"email": "An account with this email already exists."}))
			return
		}
		httpx.Fail(c, err)
		return
	}
	h.respondSession(c, http.StatusCreated, u)
}

func (h *Handler) login(c *gin.Context) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	email := normalizeEmail(in.Email)
	if !validEmail(email) || in.Password == "" || len(in.Password) > maxPasswordBytes {
		httpx.Fail(c, errBadCredentials)
		return
	}
	// Per-account limit stops password spraying from many IPs at one email.
	if err := h.limiter.Check(c, "auth-email:"+email, 10, time.Hour); err != nil {
		httpx.Fail(c, err)
		return
	}
	u, err := h.users.ByEmail(c.Request.Context(), email)
	if errors.Is(err, errNoUser) {
		_ = bcrypt.CompareHashAndPassword(h.dummyHash, []byte(in.Password))
		httpx.Fail(c, errBadCredentials)
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password)) != nil {
		httpx.Fail(c, errBadCredentials)
		return
	}
	h.respondSession(c, http.StatusOK, u)
}

func (h *Handler) logout(c *gin.Context) {
	if err := h.users.BumpTokenVersion(c.Request.Context(), CurrentUser(c).ID); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) me(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"user": CurrentUser(c)})
}

func (h *Handler) respondSession(c *gin.Context, status int, u *User) {
	tok, exp, err := h.tokens.Issue(u)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(status, session{Token: tok, ExpiresAt: exp, User: u})
}

// Required rejects requests without a valid, unrevoked bearer token.
func (h *Handler) Required() gin.HandlerFunc {
	return func(c *gin.Context) {
		u, err := h.authenticate(c)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if u == nil {
			httpx.Fail(c, apperr.ErrUnauthorized)
			return
		}
		c.Set(userKey, u)
		c.Next()
	}
}

// Optional attaches the user when a valid token is present. A bad or stale token is
// treated as anonymous rather than an error, since these routes are public.
func (h *Handler) Optional() gin.HandlerFunc {
	return func(c *gin.Context) {
		u, err := h.authenticate(c)
		if err != nil {
			if _, isApp := apperr.As(err); !isApp {
				httpx.Fail(c, err)
				return
			}
		}
		if u != nil {
			c.Set(userKey, u)
		}
		c.Next()
	}
}

// authenticate returns (nil, nil) when no token was sent.
func (h *Handler) authenticate(c *gin.Context) (*User, error) {
	header := c.GetHeader("Authorization")
	if header == "" {
		return nil, nil
	}
	raw, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return nil, apperr.ErrUnauthorized
	}
	id, tv, err := h.tokens.Parse(raw)
	if err != nil {
		return nil, apperr.ErrUnauthorized
	}
	u, err := h.users.ByID(c.Request.Context(), id)
	if errors.Is(err, errNoUser) {
		return nil, apperr.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if u.TokenVersion != tv {
		return nil, apperr.ErrUnauthorized
	}
	return u, nil
}

// CurrentUser returns the authenticated user, or nil on routes using Optional.
func CurrentUser(c *gin.Context) *User {
	u, _ := c.Get(userKey)
	user, _ := u.(*User)
	return user
}
