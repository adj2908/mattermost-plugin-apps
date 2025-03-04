package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/mattermost/mattermost-plugin-apps/apps"
	"github.com/mattermost/mattermost-plugin-apps/server/config"
	"github.com/mattermost/mattermost-plugin-apps/upstream/upaws"
	"github.com/mattermost/mattermost-plugin-apps/utils"
)

func init() {
	rootCmd.AddCommand(awsCmd)

	// init
	awsCmd.AddCommand(awsInitCmd)
	awsInitCmd.Flags().BoolVar(&shouldCreate, "create", false, "Create resources (user, group, policy, bucket) that don't already exist, using the default configuration.")
	awsInitCmd.Flags().BoolVar(&shouldCreateAccessKey, "create-access-key", false, "Create new access key for the user (or you can safely do it in AWS Console).")
	awsInitCmd.Flags().StringVar(&userName, "user", upaws.DefaultUserName, "Username to use for invoking the AWS App from Mattermost Server.")
	awsInitCmd.Flags().StringVar(&policyName, "policy", upaws.DefaultPolicyName, "Name of the policy to control access of AWS services directly by Mattermost Server (user).")
	awsInitCmd.Flags().StringVar(&groupName, "group", upaws.DefaultGroupName, "Name of the user group connecting the invoking user to the invoke policy.")

	// deploy
	awsCmd.AddCommand(awsDeployCmd)
	awsDeployCmd.Flags().BoolVar(&install, "install", false, "Install the deployed App to Mattermost")
	awsDeployCmd.Flags().BoolVar(&shouldUpdate, "update", false, "Update functions if they already exist. Use with caution in production.")
	awsDeployCmd.Flags().StringVar(&invokePolicyName, "policy", upaws.DefaultPolicyName, "name of the policy used to invoke Apps on AWS.")
	awsDeployCmd.Flags().StringVar(&executeRoleName, "execute-role", upaws.DefaultExecuteRoleName, "name of the role to be assumed by running Lambdas.")
	awsDeployCmd.Flags().StringToStringVar(&environment, "env", nil, "environment variables to pass to the App")

	// clean
	awsCmd.AddCommand(awsCleanCmd)

	// test
	awsCmd.AddCommand(awsTestCmd)
	awsTestCmd.AddCommand(awsTestLambdaCmd)
	awsTestCmd.AddCommand(awsTestDeployCmd)
	awsTestCmd.AddCommand(awsTestS3Cmd)
	awsTestCmd.AddCommand(awsTestS3ListCmd)
}

var awsCmd = &cobra.Command{
	Use:   "aws",
	Short: "Manage AWS upstream for Mattermost Apps",
}

var awsInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize AWS to deploy Mattermost Apps",
	RunE: func(cmd *cobra.Command, args []string) error {
		asDeploy, err := makeDeployAWSClient()
		if err != nil {
			return err
		}

		out, err := upaws.InitializeAWS(asDeploy, log, upaws.InitParams{
			Bucket:                upaws.S3BucketName(),
			User:                  upaws.Name(userName),
			Group:                 upaws.Name(groupName),
			Policy:                upaws.Name(policyName),
			ExecuteRole:           upaws.Name(executeRoleName),
			ShouldCreate:          shouldCreate,
			ShouldCreateAccessKey: shouldCreateAccessKey,
		})
		if err != nil {
			return err
		}

		fmt.Printf("Ready to deploy AWS Lambda Mattermost Apps!\n\n")

		fmt.Printf("User:\t%q\n", out.UserARN)
		fmt.Printf("Group:\t%q\n", out.GroupARN)
		fmt.Printf("Policy:\t%q\n", out.PolicyARN)
		fmt.Printf("Bucket:\t%q\n", out.Bucket)

		if shouldCreateAccessKey {
			fmt.Printf("\nPlease store the Access Key securely, it will not be viewable again.\n\n")
			fmt.Printf("export %s='%s'\n", upaws.AccessEnvVar, out.AccessKeyID)
			fmt.Printf("export %s='%s'\n", upaws.SecretEnvVar, out.AccessKeySecret)
		}

		return nil
	},
}

var awsCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Delete group, user and policy used for Mattermost Apps",
	RunE: func(cmd *cobra.Command, args []string) error {
		asDeploy, err := makeDeployAWSClient()
		if err != nil {
			return err
		}

		accessKeyID := os.Getenv(upaws.AccessEnvVar)
		if accessKeyID == "" {
			return errors.Errorf("no AWS access key was provided. Please set %s", upaws.AccessEnvVar)
		}

		return upaws.CleanAWS(asDeploy, accessKeyID, log)
	},
}

var awsDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy a Mattermost app to AWS (Lambda, S3)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		asDeploy, err := makeDeployAWSClient()
		if err != nil {
			return err
		}

		bucket := upaws.S3BucketName()
		out, err := upaws.DeployAppFromFile(asDeploy, args[0], log, upaws.DeployAppParams{
			Bucket:           bucket,
			InvokePolicyName: upaws.Name(invokePolicyName),
			ExecuteRoleName:  upaws.Name(executeRoleName),
			ShouldUpdate:     shouldUpdate,
			Environment:      environment,
		})
		if err != nil {
			return err
		}

		if err = updateMattermost(out.Manifest, apps.DeployAWSLambda, install); err != nil {
			return err
		}

		fmt.Printf("\n'%s' is now deployed to AWS.\n", out.Manifest.DisplayName)
		fmt.Printf("Created/updated %v functions in AWS Lambda, %v static assets in S3\n\n",
			len(out.LambdaARNs), len(out.StaticARNs))

		fmt.Printf("Execute role:\t%s\n", out.ExecuteRoleARN)
		fmt.Printf("Execute policy:\t%s\n", out.ExecutePolicyARN)
		fmt.Printf("Invoke policy:\t%s\n\n", out.InvokePolicyARN)
		fmt.Printf("Invoke policy document:\n%s\n", out.InvokePolicyDoc)
		fmt.Printf("\n")

		if !install {
			fmt.Printf("You can now install it in Mattermost using:\n")
			fmt.Printf("  /apps install listed %s\n\n", out.Manifest.AppID)
		}
		return nil
	},
}

var awsTestCmd = &cobra.Command{
	Use:   "test",
	Short: "test accessing a deployed resource",
}

func helloServerless() apps.App {
	return apps.App{
		DeployType: apps.DeployAWSLambda,
		Manifest: apps.Manifest{
			AppID:   "hello-serverless",
			Version: "v1.1.0",
			Deploy: apps.Deploy{
				AWSLambda: &apps.AWSLambda{
					Functions: []apps.AWSLambdaFunction{
						{
							Path:    "/",
							Name:    "hello-serverless",
							Handler: "hello-serverless",
							Runtime: "go1.x",
						},
					},
				},
			},
		},
	}
}

var awsTestS3Cmd = &cobra.Command{
	Use:   "s3",
	Short: "test accessing a static S3 resource",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		upTest, err := makeTestAWSUpstream()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), config.RequestTimeout)
		defer cancel()

		resp, _, err := upTest.GetStatic(ctx, helloServerless(), "test.txt")
		if err != nil {
			return err
		}
		defer resp.Close()
		data, err := io.ReadAll(resp)
		if err != nil {
			return err
		}
		r := string(data)
		log.Debugw("received data", "data", string(data))

		if r != "static pong" {
			return errors.Errorf("expected 'static pong', got '%s'", r)
		}
		fmt.Println("OK")
		return nil
	},
}

var awsTestS3ListCmd = &cobra.Command{
	Use:   "list",
	Short: "test listing S3 manifests",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		upTest, err := makeTestAWSUpstream()
		if err != nil {
			return err
		}

		resp, err := upTest.ListS3Apps("hello")
		if err != nil {
			return err
		}
		fmt.Println(resp)
		return nil
	},
}

var awsTestLambdaCmd = &cobra.Command{
	Use:   "lambda",
	Short: "test accessing hello-serverless /ping function",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		upTest, err := makeTestAWSUpstream()
		if err != nil {
			return err
		}

		creq := apps.CallRequest{
			Call: *apps.NewCall("/ping"),
		}

		ctx, cancel := context.WithTimeout(context.Background(), config.RequestTimeout)
		defer cancel()

		resp, err := upTest.Roundtrip(ctx, helloServerless(), creq, false)
		if err != nil {
			return err
		}
		defer resp.Close()

		data, err := io.ReadAll(resp)
		if err != nil {
			return err
		}
		log.Debugf("Received: %s", string(data))

		cresp := apps.CallResponse{}
		_ = json.Unmarshal(data, &cresp)
		expected := apps.NewTextResponse("PONG")
		if cresp != expected {
			return errors.Errorf("invalid value received: %s", string(data))
		}

		fmt.Println("OK")
		return nil
	},
}

var awsTestDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "deploys 'hello-serverless' app for testing",
	Long: `Test commands us the 'hello-serverless' example app for testing, see
https://github.com/mattermost/mattermost-plugin-apps/tree/master/examples/go/hello-serverless/README.md

The App needs to be built with 'make dist' in its own directory, then use

	appsctl aws test deploy <dist-bundle-path>

command to deploy it to AWS. This command is equivalent to

	appsctl aws deploy app <dist-bundle-path> --update

with the default initial IAM configuration`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		bundlePath := args[0]

		asDeploy, err := makeDeployAWSClient()
		if err != nil {
			return err
		}

		out, err := upaws.DeployAppFromFile(asDeploy, bundlePath, log, upaws.DeployAppParams{
			Bucket:           upaws.S3BucketName(),
			InvokePolicyName: upaws.Name(upaws.DefaultPolicyName),
			ExecuteRoleName:  upaws.Name(upaws.DefaultExecuteRoleName),
			ShouldUpdate:     true,
		})
		if err != nil {
			return err
		}

		fmt.Printf("Success!\n\n%s\n", utils.Pretty(out))
		return nil
	},
}

func makeTestAWSUpstream() (*upaws.Upstream, error) {
	region := os.Getenv(upaws.RegionEnvVar)
	if region == "" {
		return nil, errors.Errorf("no AWS region was provided. Please set %s", upaws.RegionEnvVar)
	}
	accessKey := os.Getenv(upaws.AccessEnvVar)
	if accessKey == "" {
		return nil, errors.Errorf("no AWS access key was provided. Please set %s", upaws.AccessEnvVar)
	}
	secretKey := os.Getenv(upaws.SecretEnvVar)
	if secretKey == "" {
		return nil, errors.Errorf("no AWS secret key was provided. Please set %s", upaws.SecretEnvVar)
	}

	return upaws.MakeUpstream(accessKey, secretKey, region, upaws.S3BucketName(), log)
}

func makeDeployAWSClient() (upaws.Client, error) {
	region := os.Getenv(upaws.RegionEnvVar)
	if region == "" {
		return nil, errors.Errorf("no AWS region was provided. Please set %s", upaws.RegionEnvVar)
	}
	accessKey := os.Getenv(upaws.DeployAccessEnvVar)
	if accessKey == "" {
		return nil, errors.Errorf("no AWS access key was provided. Please set %s", upaws.DeployAccessEnvVar)
	}
	secretKey := os.Getenv(upaws.DeploySecretEnvVar)
	if secretKey == "" {
		return nil, errors.Errorf("no AWS secret key was provided. Please set %s", upaws.DeploySecretEnvVar)
	}

	return upaws.MakeClient(accessKey, secretKey, region,
		log.With("purpose", "appsctl deploy"))
}
