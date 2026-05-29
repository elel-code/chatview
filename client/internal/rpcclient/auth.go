package rpcclient

import (
	"context"
	"errors"

	authpb "chatview/api/gen/chatview/auth"
	"chatview/client/internal/domain"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (c *Client) Login(ctx context.Context, publicKeyHex string, sign func([]byte) []byte) (domain.LoginResult, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	challenge, err := c.auth.RequestChallenge(ctx, &authpb.RequestChallengeReq{PublicKey: publicKeyHex})
	if err != nil {
		return domain.LoginResult{}, rpcError(err)
	}
	signature := sign(challenge.Challenge)
	resp, err := c.auth.Login(ctx, &authpb.LoginReq{
		PublicKey:          publicKeyHex,
		ChallengeSignature: signature,
	})
	if err != nil {
		if status.Code(err) == codes.PermissionDenied {
			return domain.LoginResult{}, errors.New("account banned")
		}
		return domain.LoginResult{}, rpcError(err)
	}

	c.mu.Lock()
	c.sessionToken = resp.SessionToken
	c.publicKey = resp.PublicKey
	c.mu.Unlock()

	return domain.LoginResult{PublicKey: resp.PublicKey, Role: resp.Role}, nil
}
