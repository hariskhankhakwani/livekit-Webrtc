package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/twitchtv/twirp"

	"github.com/livekit/livekit-server/pkg/auth"
)

const (
	authorizationHeader = "Authorization"
	bearerPrefix        = "Bearer "
	grantsKey           = "grants"
)

var (
	ErrPermissionDenied = errors.New("permissions denied")
	AuthRequired        bool
)

// authentication middleware
type APIKeyAuthMiddleware struct {
	provider auth.KeyProvider
}

func NewAPIKeyAuthMiddleware(provider auth.KeyProvider) *APIKeyAuthMiddleware {
	return &APIKeyAuthMiddleware{
		provider: provider,
	}
}

func (m *APIKeyAuthMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	authHeader := r.Header.Get(authorizationHeader)

	if authHeader != "" {
		if !strings.HasPrefix(authHeader, bearerPrefix) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("invalid authorization header. Must start with " + bearerPrefix))
			return
		}

		authToken := authHeader[len(bearerPrefix):]
		v, err := auth.ParseAPIToken(authToken)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("invalid authorization token"))
			return
		}

		secret := m.provider.GetSecret(v.APIKey())
		if secret == "" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("invalid API key"))
			return
		}

		grants, err := v.Verify(secret)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("invalid token: " + err.Error()))
			return
		}

		// set grants in context
		ctx := r.Context()
		r = r.WithContext(context.WithValue(ctx, grantsKey, grants))
	}

	next.ServeHTTP(w, r)
}

func GetGrants(ctx context.Context) *auth.ClaimGrants {
	claims, ok := ctx.Value(grantsKey).(*auth.ClaimGrants)
	if !ok {
		return nil
	}
	return claims
}

func SetAuthorizationToken(r *http.Request, token string) {
	r.Header.Set(authorizationHeader, bearerPrefix+token)
}

func EnsureJoinPermission(ctx context.Context) (name string, err error) {
	if !AuthRequired {
		return "", nil
	}
	claims := GetGrants(ctx)
	if claims == nil || claims.Video == nil {
		err = ErrPermissionDenied
		return
	}

	if claims.Video.RoomJoin {
		name = claims.Video.Room
	} else {
		err = ErrPermissionDenied
	}
	return
}

func EnsureCreatePermission(ctx context.Context) error {
	if !AuthRequired {
		return nil
	}
	claims := GetGrants(ctx)
	if claims == nil {
		return ErrPermissionDenied
	}

	if claims.Video.RoomCreate {
		return nil
	}
	return ErrPermissionDenied
}

// wraps authentication errors around Twirp
func twirpAuthError(err error) error {
	return twirp.NewError(twirp.Unauthenticated, err.Error())
}