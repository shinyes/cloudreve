package inventory

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/oauthclient"
	"github.com/cloudreve/Cloudreve/v4/ent/oauthgrant"
)

type (
	OAuthClientClient interface {
		TxOperator
		// GetByGUID returns the OAuth client by its GUID (client_id).
		GetByGUID(ctx context.Context, guid string) (*ent.OAuthClient, error)
		// GetByGUIDWithGrants returns the OAuth client by its GUID (client_id) with the grants for the user.
		GetByGUIDWithGrants(ctx context.Context, guid string, uid int) (*ent.OAuthClient, error)
		// UpsertGrant creates or updates an OAuth grant for a user and client.
		UpsertGrant(ctx context.Context, userID, clientID int, scopes []string) error
		// UpdateGrantLastUsedAt updates the last used at for an OAuth grant for a user and client.
		UpdateGrantLastUsedAt(ctx context.Context, userID, clientID int) error
	}
)

func NewOAuthClientClient(client *ent.Client) OAuthClientClient {
	return &oauthClientClient{
		client: client,
	}
}

type oauthClientClient struct {
	client *ent.Client
}

func (c *oauthClientClient) SetClient(newClient *ent.Client) TxOperator {
	return &oauthClientClient{client: newClient}
}

func (c *oauthClientClient) GetClient() *ent.Client {
	return c.client
}

func (c *oauthClientClient) GetByGUID(ctx context.Context, guid string) (*ent.OAuthClient, error) {
	return c.client.OAuthClient.Query().
		Where(oauthclient.GUID(guid), oauthclient.IsEnabled(true)).
		First(ctx)
}

func (c *oauthClientClient) GetByGUIDWithGrants(ctx context.Context, guid string, uid int) (*ent.OAuthClient, error) {
	stm := c.client.OAuthClient.Query().
		Where(oauthclient.GUID(guid), oauthclient.IsEnabled(true))
	if uid > 0 {
		stm.WithGrants(func(ogq *ent.OAuthGrantQuery) {
			ogq.Where(oauthgrant.UserID(uid))
		})
	}

	return stm.First(ctx)
}

func (c *oauthClientClient) UpsertGrant(ctx context.Context, userID, clientID int, scopes []string) error {
	return c.client.OAuthGrant.Create().
		SetUserID(userID).
		SetClientID(clientID).
		SetScopes(scopes).
		SetLastUsedAt(time.Now()).
		OnConflict(
			sql.ConflictColumns(oauthgrant.FieldUserID, oauthgrant.FieldClientID),
		).
		UpdateScopes().
		UpdateLastUsedAt().
		Exec(ctx)
}

func (c *oauthClientClient) UpdateGrantLastUsedAt(ctx context.Context, userID, clientID int) error {
	return c.client.OAuthGrant.Update().
		Where(oauthgrant.UserID(userID), oauthgrant.ClientID(clientID)).
		SetLastUsedAt(time.Now()).
		Exec(ctx)
}
