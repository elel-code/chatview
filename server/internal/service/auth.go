package service

import (
	"context"
	"crypto/ed25519"
	"time"

	authpb "chatview/api/gen/chatview/auth"
	eventspb "chatview/api/gen/chatview/events"
	"chatview/server/internal/auth"
	"chatview/server/internal/db"
	"chatview/server/internal/eventhub"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AuthService struct {
	authpb.UnimplementedAuthServiceServer
	Store        *db.Store
	Hub          *eventhub.Hub
	ChallengeTTL time.Duration
	SessionTTL   time.Duration
}

func (s *AuthService) RequestChallenge(ctx context.Context, req *authpb.RequestChallengeReq) (*authpb.RequestChallengeResp, error) {
	publicKey := req.GetPublicKey()
	if _, err := auth.ParseEd25519PublicKey(publicKey); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid public_key format")
	}
	challenge := auth.RandomBytes(32)
	expiresAt := time.Now().UTC().Add(s.ChallengeTTL)

	tx, err := s.Store.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	defer tx.Rollback()

	var createdUser bool
	result, err := tx.ExecContext(ctx, `
		INSERT INTO users (pub_key) VALUES ($1)
		ON CONFLICT (pub_key) DO NOTHING
	`, publicKey)
	if err == nil {
		if rows, rowsErr := result.RowsAffected(); rowsErr == nil && rows > 0 {
			createdUser = true
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO challenges (pub_key, challenge, expires_at)
			VALUES ($1, $2, $3)
			ON CONFLICT (pub_key) DO UPDATE
			SET challenge = EXCLUDED.challenge, expires_at = EXCLUDED.expires_at, created_at = now()
		`, publicKey, challenge, expiresAt)
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	if err := tx.Commit(); err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	if createdUser && s.Hub != nil {
		s.Hub.PushAdmins(ctx, s.Store, &eventspb.ServerEvent{Event: &eventspb.ServerEvent_AdminUpdate{
			AdminUpdate: &eventspb.AdminUpdateEvent{},
		}})
	}
	return &authpb.RequestChallengeResp{Challenge: challenge, ExpiresAt: expiresAt.Unix()}, nil
}

func (s *AuthService) Login(ctx context.Context, req *authpb.LoginReq) (*authpb.LoginResp, error) {
	publicKey := req.GetPublicKey()
	pubKey, err := auth.ParseEd25519PublicKey(publicKey)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid public_key format")
	}
	if len(req.GetChallengeSignature()) != ed25519.SignatureSize {
		return nil, status.Error(codes.Unauthenticated, "invalid signature")
	}

	tx, err := s.Store.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	defer tx.Rollback()

	var challenge []byte
	var role int32
	var userStatus int32
	err = tx.QueryRowxContext(ctx, `
		SELECT c.challenge, u.role, u.status
		FROM challenges c
		JOIN users u ON u.pub_key = c.pub_key
		WHERE c.pub_key = $1 AND c.expires_at > now()
	`, publicKey).Scan(&challenge, &role, &userStatus)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "challenge expired")
	}
	if !ed25519.Verify(pubKey, challenge, req.GetChallengeSignature()) {
		return nil, status.Error(codes.Unauthenticated, "invalid signature")
	}
	if userStatus == 2 {
		return nil, status.Error(codes.PermissionDenied, "account banned")
	}
	token := auth.NewToken()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO sessions (token, pub_key, expires_at, is_online)
		VALUES ($1, $2, $3, false)
	`, token, publicKey, db.SessionExpires(s.SessionTTL))
	if err == nil {
		_, err = tx.ExecContext(ctx, `DELETE FROM challenges WHERE pub_key = $1`, publicKey)
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	if err := tx.Commit(); err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	return &authpb.LoginResp{SessionToken: token, Role: role, PublicKey: publicKey}, nil
}
