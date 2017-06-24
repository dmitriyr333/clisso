package onelogin

import (
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/howeyc/gopass"
)

// TODO Allow configuration using config file
// TODO Allow configuration from CLI (CLI > env var > config file)

func Get() {
	// Get env vars
	var secret string = os.Getenv("ONELOGIN_CLIENT_SECRET")
	var id string = os.Getenv("ONELOGIN_CLIENT_ID")
	var appId string = os.Getenv("ONELOGIN_APP_ID")
	var principal string = os.Getenv("ONELOGIN_PRINCIPAL_ARN")
	var role string = os.Getenv("ONELOGIN_ROLE_ARN")
	var subdomain string = os.Getenv("ONELOGIN_SUBDOMAIN")

	if secret == "" {
		log.Fatal("The ONELOGIN_CLIENT_SECRET environment variable must bet set")
	}
	if id == "" {
		log.Fatal("The ONELOGIN_CLIENT_ID environment variable must bet set")
	}
	if appId == "" {
		log.Fatal("The ONELOGIN_APP_ID environment variable must bet set")
	}
	if principal == "" {
		log.Fatal("The ONELOGIN_PRINCIPAL_ARN environment variable must bet set")
	}
	if role == "" {
		log.Fatal("The ONELOGIN_ROLE_ARN environment variable must bet set")
	}
	if subdomain == "" {
		log.Fatal("The ONELOGIN_SUBDOMAIN environment variable must bet set")
	}

	// Get OneLogin access token
	log.Println("Generating OneLogin access tokens")
	token, err := GenerateTokens(GenerateTokensUrl, id, secret)
	if err != nil {
		log.Fatal(err)
	}

	// Get credentials from the user
	fmt.Print("OneLogin username: ")
	var user string
	fmt.Scanln(&user)

	fmt.Print("OneLogin password: ")
	pass, err := gopass.GetPasswd()
	if err != nil {
		log.Fatal("Couldn't read password from terminal")
	}

	// Generate SAML assertion
	log.Println("Generating SAML assertion")
	pSaml := GenerateSamlAssertionParams{
		UsernameOrEmail: user,
		Password:        string(pass),
		AppId:           appId,
		// TODO At the moment when there is a mismatch between Subdomain and
		// the domain in the username, the user is getting HTTP 400.
		Subdomain: subdomain,
	}

	rSaml, err := GenerateSamlAssertion(
		GenerateSamlAssertionUrl, token, &pSaml,
	)
	if err != nil {
		log.Fatal(err)
	}

	st := rSaml.Data[0].StateToken

	devices := rSaml.Data[0].Devices

	var deviceId string
	if len(devices) > 1 {
		for i, d := range devices {
			fmt.Printf("%d. %d - %s\n", i+1, d.DeviceId, d.DeviceType)
		}

		fmt.Printf("Please choose an MFA device to authenticate with (1-%d): ", len(devices))
		var selection int
		fmt.Scanln(&selection)

		deviceId = fmt.Sprintf("%v", devices[selection-1].DeviceId)
	} else {
		deviceId = fmt.Sprintf("%v", devices[0].DeviceId)
	}

	fmt.Print("Please enter the OTP from your MFA device: ")
	var otp string
	fmt.Scanln(&otp)

	// Verify MFA
	pMfa := VerifyFactorParams{
		AppId:      appId,
		DeviceId:   deviceId,
		StateToken: st,
		OtpToken:   otp,
	}

	rMfa, err := VerifyFactor(VerifyFactorUrl, token, &pMfa)
	if err != nil {
		log.Fatal(err)
	}

	samlAssertion := rMfa.Data

	// Assume role
	pAssumeRole := sts.AssumeRoleWithSAMLInput{
		PrincipalArn:  aws.String(principal),
		RoleArn:       aws.String(role),
		SAMLAssertion: aws.String(samlAssertion),
	}

	sess := session.Must(session.NewSession())
	svc := sts.New(sess)

	resp, err := svc.AssumeRoleWithSAML(&pAssumeRole)
	if err != nil {
		log.Fatal(err)
	}

	keyId := *resp.Credentials.AccessKeyId
	secretKey := *resp.Credentials.SecretAccessKey
	sessionToken := *resp.Credentials.SessionToken

	// Set temporary credentials in environment
	// TODO Error if already set
	// TODO Write vars to creds file
	fmt.Println("Paste the following in your terminal:")
	fmt.Println()
	fmt.Printf("export AWS_ACCESS_KEY_ID=%v\n", keyId)
	fmt.Printf("export AWS_SECRET_ACCESS_KEY=%v\n", secretKey)
	fmt.Printf("export AWS_SESSION_TOKEN=%v\n", sessionToken)
}