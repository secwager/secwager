package internal

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
)

// UserManager abstracts Cognito so tests can inject a fake.
type UserManager interface {
	CreateUser(ctx context.Context, username, email string) (cognitoSub string, err error)
	DeleteUser(ctx context.Context, username string) error
}

// CognitoUserManager is the production UserManager backed by AWS Cognito.
type CognitoUserManager struct {
	client *cognitoidentityprovider.Client
	poolID string
}

func NewCognitoUserManager(client *cognitoidentityprovider.Client, poolID string) *CognitoUserManager {
	return &CognitoUserManager{client: client, poolID: poolID}
}

func (m *CognitoUserManager) CreateUser(ctx context.Context, username, email string) (string, error) {
	out, err := m.client.AdminCreateUser(ctx, &cognitoidentityprovider.AdminCreateUserInput{
		UserPoolId:    aws.String(m.poolID),
		Username:      aws.String(username),
		MessageAction: types.MessageActionTypeSuppress,
		UserAttributes: []types.AttributeType{
			{Name: aws.String("email"), Value: aws.String(email)},
		},
	})
	if err != nil {
		var uee *types.UsernameExistsException
		if errors.As(err, &uee) {
			return "", &AlreadyExistsError{Msg: "username already exists in Cognito: " + username}
		}
		return "", fmt.Errorf("cognito AdminCreateUser: %w", err)
	}
	for _, attr := range out.User.Attributes {
		if aws.ToString(attr.Name) == "sub" {
			return aws.ToString(attr.Value), nil
		}
	}
	return "", fmt.Errorf("cognito created user %s but sub attribute missing", username)
}

func (m *CognitoUserManager) DeleteUser(ctx context.Context, username string) error {
	_, err := m.client.AdminDeleteUser(ctx, &cognitoidentityprovider.AdminDeleteUserInput{
		UserPoolId: aws.String(m.poolID),
		Username:   aws.String(username),
	})
	if err != nil {
		return fmt.Errorf("cognito AdminDeleteUser: %w", err)
	}
	return nil
}
