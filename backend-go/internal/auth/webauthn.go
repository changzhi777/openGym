// WebAuthn 封装 — go-webauthn;options/credential 的 JSON 形状与前端
// lib/api.js(base64url 字符串字段)和 @simplewebauthn 的 Node 版逐字段兼容。
package auth

import (
	"encoding/base64"
	"errors"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

type WebAuthn struct {
	wa *webauthn.WebAuthn
}

func NewWebAuthn(rpName, rpID, origin string) (*WebAuthn, error) {
	w, err := webauthn.New(&webauthn.Config{
		RPDisplayName: rpName,
		RPID:          rpID,
		RPOrigins:     []string{origin},
	})
	if err != nil {
		return nil, err
	}
	return &WebAuthn{wa: w}, nil
}

// KeyHandle 是 store 里一把 passkey 的最小形状(与 passkeys 表对应)。
type KeyHandle struct {
	ID        string // credential id(base64url)
	PublicKey []byte // COSE
	Counter   uint32
}

// waUser 适配 go-webauthn 的 User 接口(仅用于仪式,数据来自 store)。
type waUser struct {
	id   []byte
	name string
	keys []webauthn.Credential
}

func (u *waUser) WebAuthnID() []byte                 { return u.id }
func (u *waUser) WebAuthnName() string               { return u.name }
func (u *waUser) WebAuthnDisplayName() string        { return u.name }
func (u *waUser) WebAuthnCredentials() []webauthn.Credential { return u.keys }

func toCred(k KeyHandle) webauthn.Credential {
	id, _ := base64.RawURLEncoding.DecodeString(k.ID)
	return webauthn.Credential{ID: id, PublicKey: k.PublicKey, Authenticator: webauthn.Authenticator{SignCount: k.Counter}}
}

// CreationOptions 生成注册 options;返回 (给前端的 JSON, challenge base64url)。
func (w *WebAuthn) CreationOptions(uid []byte, name string) (any, string, error) {
	u := &waUser{id: uid, name: name}
	opts, session, err := w.wa.BeginRegistration(u,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationPreferred,
		}),
	)
	if err != nil {
		return nil, "", err
	}
	return opts.Response, session.Challenge, nil
}

// RequestOptions 生成登录 options(发现式 passkey:与 Node 的 allowCredentials:[] 同形状)。
func (w *WebAuthn) RequestOptions() (any, string, error) {
	opts, session, err := w.wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationPreferred))
	if err != nil {
		return nil, "", err
	}
	return opts.Response, session.Challenge, nil
}

var ErrVerify = errors.New("verify-error")

// VerifyRegistration 校验注册响应;成功返回新凭据。
func (w *WebAuthn) VerifyRegistration(uid []byte, name string, body []byte, expectedChallenge string) (KeyHandle, error) {
	parsed, err := protocol.ParseCredentialCreationResponseBody(newBodyReader(body))
	if err != nil {
		return KeyHandle{}, ErrVerify
	}
	u := &waUser{id: uid, name: name}
	session := webauthn.SessionData{
		Challenge: expectedChallenge,
		UserID:    uid,
	}
	cred, err := w.wa.CreateCredential(u, session, parsed)
	if err != nil {
		return KeyHandle{}, ErrVerify
	}
	return KeyHandle{
		ID:        base64.RawURLEncoding.EncodeToString(cred.ID),
		PublicKey: cred.PublicKey,
		Counter:   0,
	}, nil
}

// VerifyAuthentication 校验发现式登录响应;findUser 由 handler 传入
// (assertion 的 userHandle → 该用户的凭据集)。返回新计数器。
func (w *WebAuthn) VerifyAuthentication(uid []byte, name string, keys []KeyHandle, body []byte, expectedChallenge string) (uint32, error) {
	parsed, err := protocol.ParseCredentialRequestResponseBody(newBodyReader(body))
	if err != nil {
		return 0, ErrVerify
	}
	creds := make([]webauthn.Credential, len(keys))
	for i, k := range keys {
		creds[i] = toCred(k)
	}
	u := &waUser{id: uid, name: name, keys: creds}
	session := webauthn.SessionData{Challenge: expectedChallenge}
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		if len(userHandle) > 0 && string(userHandle) != string(uid) {
			return nil, ErrVerify
		}
		return u, nil
	}
	_, err = w.wa.ValidateDiscoverableLogin(handler, session, parsed)
	if err != nil {
		return 0, ErrVerify
	}
	return parsed.Response.AuthenticatorData.Counter, nil
}

func decodeB64u(s string) []byte {
	b, _ := base64.RawURLEncoding.DecodeString(s)
	return b
}

// CreationOptionsExclude 加第二把 passkey 的 options:exclude 现有凭据
// (authenticator 持有时会提示而非再造一把;与 Node moreOptions 同义)。
func (w *WebAuthn) CreationOptionsExclude(uid []byte, name string, existing []KeyHandle) (any, string, error) {
	u := &waUser{id: uid, name: name}
	excl := make([]protocol.CredentialDescriptor, 0, len(existing))
	for _, k := range existing {
		id, _ := base64.RawURLEncoding.DecodeString(k.ID)
		excl = append(excl, protocol.CredentialDescriptor{CredentialID: id})
	}
	opts, session, err := w.wa.BeginRegistration(u,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationPreferred,
		}),
		webauthn.WithExclusions(excl),
	)
	if err != nil {
		return nil, "", err
	}
	return opts.Response, session.Challenge, nil
}
