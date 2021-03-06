package operator2

import (
	"context"
	"crypto/rand"
	"encoding/base64"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	oauthv1 "github.com/openshift/api/oauth/v1"
	oauthclient "github.com/openshift/client-go/oauth/clientset/versioned/typed/oauth/v1"
	"github.com/openshift/library-go/pkg/oauth/oauthdiscovery"
)

// ensureBootstrappedOAuthClients creates or updates the bootstrap oauth clients that openshift relies upon.
func (c *authOperator) ensureBootstrappedOAuthClients(ctx context.Context, masterPublicURL string) error {
	browserClient := oauthv1.OAuthClient{
		ObjectMeta:            metav1.ObjectMeta{Name: "openshift-browser-client"},
		Secret:                random256BitsString(),
		RespondWithChallenges: false,
		RedirectURIs:          []string{oauthdiscovery.OpenShiftOAuthTokenDisplayURL(masterPublicURL)},
		GrantMethod:           oauthv1.GrantHandlerAuto,
	}
	if err := ensureOAuthClient(ctx, c.oauthClientClient, browserClient); err != nil {
		return err
	}

	cliClient := oauthv1.OAuthClient{
		ObjectMeta:            metav1.ObjectMeta{Name: "openshift-challenging-client"},
		Secret:                "",
		RespondWithChallenges: true,
		RedirectURIs:          []string{oauthdiscovery.OpenShiftOAuthTokenImplicitURL(masterPublicURL)},
		GrantMethod:           oauthv1.GrantHandlerAuto,
	}
	if err := ensureOAuthClient(ctx, c.oauthClientClient, cliClient); err != nil {
		return err
	}

	return nil
}

func ensureOAuthClient(ctx context.Context, oauthClients oauthclient.OAuthClientInterface, client oauthv1.OAuthClient) error {
	_, err := oauthClients.Create(ctx, &client, metav1.CreateOptions{})
	if err == nil || !apierrors.IsAlreadyExists(err) {
		return err
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing, err := oauthClients.Get(ctx, client.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		existingCopy := existing.DeepCopy()

		if len(client.Secret) == 0 {
			existingCopy.Secret = ""
		}
		if len(existingCopy.Secret) < len(client.Secret) {
			existingCopy.Secret = client.Secret
		}

		existingCopy.RespondWithChallenges = client.RespondWithChallenges
		existingCopy.RedirectURIs = client.RedirectURIs
		existingCopy.GrantMethod = client.GrantMethod
		existingCopy.ScopeRestrictions = client.ScopeRestrictions

		if equality.Semantic.DeepEqual(existing, existingCopy) {
			return nil
		}

		_, err = oauthClients.Update(ctx, existingCopy, metav1.UpdateOptions{})
		return err
	})
}

func randomBits(bits int) []byte {
	size := bits / 8
	if bits%8 != 0 {
		size++
	}
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		panic(err) // rand should never fail
	}
	return b
}

// RandomBitsString returns a random string with at least the requested bits of entropy.
// It uses RawURLEncoding to ensure we do not get / characters or trailing ='s.
func randomBitsString(bits int) string {
	return base64.RawURLEncoding.EncodeToString(randomBits(bits))
}

// Random256BitsString is a convenience function for calling RandomBitsString(256).
// Callers that need a random string should use this function unless they have a
// very good reason to need a different amount of entropy.
func random256BitsString() string {
	// 32 bytes (256 bits) = 43 base64-encoded characters
	return randomBitsString(256)
}
