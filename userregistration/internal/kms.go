package internal

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
)

// KeyEncryptor abstracts KMS encryption so tests can inject a fake.
type KeyEncryptor interface {
	Encrypt(ctx context.Context, plaintext []byte) (ciphertext []byte, err error)
}

// KMSEncryptor is the production KeyEncryptor backed by AWS KMS.
type KMSEncryptor struct {
	client *kms.Client
	keyARN string
}

func NewKMSEncryptor(client *kms.Client, keyARN string) *KMSEncryptor {
	return &KMSEncryptor{client: client, keyARN: keyARN}
}

func (e *KMSEncryptor) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	out, err := e.client.Encrypt(ctx, &kms.EncryptInput{
		KeyId:     aws.String(e.keyARN),
		Plaintext: plaintext,
	})
	if err != nil {
		return nil, fmt.Errorf("kms Encrypt: %w", err)
	}
	return out.CiphertextBlob, nil
}
