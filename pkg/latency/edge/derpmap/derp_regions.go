package derpmap

// maps the DERP "RegionName" to the nearest AWS region name.
// ref. https://aws.amazon.com/about-aws/global-infrastructure/regions_az/
var derpRegionNameToAWSRegion = map[string]string{
	"Amsterdam":     "eu-west-1",
	"Bangalore":     "ap-south-1",
	"Chicago":       "us-east-2",
	"Dallas":        "us-west-2",
	"Denver":        "us-west-1",
	"Dubai":         "me-central-1",
	"Frankfurt":     "eu-central-1",
	"Hong Kong":     "ap-east-1",
	"Honolulu":      "us-west-2",
	"Johannesburg":  "af-south-1",
	"London":        "eu-west-2",
	"Los Angeles":   "us-west-1",
	"Madrid":        "eu-south-2",
	"Miami":         "us-east-1",
	"Nairobi":       "af-south-1",
	"New York City": "us-east-1",
	"Paris":         "eu-west-3",
	"San Francisco": "us-west-1",
	"Seattle":       "us-west-2",
	"Singapore":     "ap-southeast-1",
	"Sydney":        "ap-southeast-2",
	"São Paulo":     "sa-east-1",
	"Tokyo":         "ap-northeast-1",
	"Toronto":       "ca-central-1",
	"Warsaw":        "eu-central-1",
}

// GetAWSRegion returns the AWS region for the given DERP region name.
func GetAWSRegion(regionName string) (string, bool) {
	region, ok := derpRegionNameToAWSRegion[regionName]
	return region, ok
}
