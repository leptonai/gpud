package imds

import "context"

// FetchInstanceType returns the EC2 instance type from IMDSv2.
func FetchInstanceType(ctx context.Context) (string, error) {
	return fetchInstanceType(ctx, imdsTokenURL, imdsMetadataURL)
}

func fetchInstanceType(ctx context.Context, tokenURL, metadataURL string) (string, error) {
	return fetchMetadataByPath(ctx, tokenURL, metadataURL+"/instance-type")
}
