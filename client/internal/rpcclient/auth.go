package rpcclient

import (
	"context"
	"errors"

	authpb "chatview/api/gen/chatview/auth"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (c *Client) Login(ctx context.Context, publicKeyHex string, sign func([]byte) []byte) (LoginResult, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	challenge, err := c.auth.RequestChallenge(ctx, &authpb.RequestChallengeReq{PubKey: publicKeyHex})
	if err != nil {
		return LoginResult{}, rpcError(err)
	}
	signature := sign(challenge.Challenge)
	resp, err := c.auth.Login(ctx, &authpb.LoginReq{
		PubKey:             publicKeyHex,
		ChallengeSignature: signature,
	})
	if err != nil {
		if status.Code(err) == codes.PermissionDenied {
			return LoginResult{}, errors.New("account banned")
		}
		return LoginResult{}, rpcError(err)
	}

	c.mu.Lock()
	c.sessionToken = resp.SessionToken
	c.publicKey = resp.PubKey
	c.mu.Unlock()

	return LoginResult{PublicKey: resp.PubKey, Role: resp.Role}, nil
}
