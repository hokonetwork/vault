package database

import (
	"fmt"

	"github.com/hashicorp/vault/logical"
	"github.com/hashicorp/vault/logical/framework"
)

const SecretCredsType = "creds"

func secretCreds(b *databaseBackend) *framework.Secret {
	return &framework.Secret{
		Type:   SecretCredsType,
		Fields: map[string]*framework.FieldSchema{},

		Renew:  b.secretCredsRenew,
		Revoke: b.secretCredsRevoke,
	}
}

func (b *databaseBackend) secretCredsRenew(req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	// Get the username from the internal data
	usernameRaw, ok := req.Secret.InternalData["username"]
	if !ok {
		return nil, fmt.Errorf("secret is missing username internal data")
	}
	username, ok := usernameRaw.(string)

	roleNameRaw, ok := req.Secret.InternalData["role"]
	if !ok {
		return nil, fmt.Errorf("could not find role with name: %s", req.Secret.InternalData["role"])
	}

	role, err := b.Role(req.Storage, roleNameRaw.(string))
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, fmt.Errorf("could not find role with name: %s", req.Secret.InternalData["role"])
	}

	f := framework.LeaseExtend(role.DefaultTTL, role.MaxTTL, b.System())
	resp, err := f(req, d)
	if err != nil {
		return nil, err
	}

	// Grab the read lock
	b.Lock()
	defer b.Unlock()

	// Get our connection
	db, err := b.getOrCreateDBObj(req.Storage, role.DBName)
	if err != nil {
		return nil, fmt.Errorf("could not find connection with name %s, got err: %s", role.DBName, err)
	}

	// Make sure we increase the VALID UNTIL endpoint for this user.
	if expireTime := resp.Secret.ExpirationTime(); !expireTime.IsZero() {
		expiration := expireTime.Format("2006-01-02 15:04:05-0700")

		err := db.RenewUser(role.Statements, username, expiration)
		if err != nil {
			return nil, err
		}
	}

	return resp, nil
}

func (b *databaseBackend) secretCredsRevoke(req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	// Get the username from the internal data
	usernameRaw, ok := req.Secret.InternalData["username"]
	if !ok {
		return nil, fmt.Errorf("secret is missing username internal data")
	}
	username, ok := usernameRaw.(string)

	var resp *logical.Response

	roleNameRaw, ok := req.Secret.InternalData["role"]
	if !ok {
		return nil, fmt.Errorf("could not find role with name: %s", req.Secret.InternalData["role"])
	}

	role, err := b.Role(req.Storage, roleNameRaw.(string))
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, fmt.Errorf("could not find role with name: %s", req.Secret.InternalData["role"])
	}

	/* TODO: think about how to handle this case.
	if !ok {
		role, err := b.Role(req.Storage, roleNameRaw.(string))
		if err != nil {
			return nil, err
		}
		if role == nil {
			if resp == nil {
				resp = &logical.Response{}
			}
			resp.AddWarning(fmt.Sprintf("Role %q cannot be found. Using default revocation SQL.", roleNameRaw.(string)))
		} else {
			revocationSQL = role.RevocationStatement
		}
	}*/

	// Grab the read lock
	b.Lock()
	defer b.Unlock()

	// Get our connection
	db, err := b.getOrCreateDBObj(req.Storage, role.DBName)
	if err != nil {
		return nil, fmt.Errorf("could not find database with name: %s, got error: %s", role.DBName, err)
	}

	err = db.RevokeUser(role.Statements, username)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
