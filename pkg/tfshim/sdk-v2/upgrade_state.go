package sdkv2

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func upgradeResourceState(p *schema.Provider, res *schema.Resource,
	instanceState *terraform.InstanceState) (*terraform.InstanceState, error) {

	m := instanceState.Attributes

	// Ensure that we have an ID in the attributes.
	m["id"] = instanceState.ID

	version, hasVersion := 0, false
	if versionValue, ok := instanceState.Meta["schema_version"]; ok {
		versionString, ok := versionValue.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected type %T for schema_version", versionValue)
		}
		v, err := strconv.ParseInt(versionString, 0, 32)
		if err != nil {
			return nil, err
		}
		version, hasVersion = int(v), true
	}

	// First, build a JSON state from the InstanceState.
	json, version, err := resource.UpgradeFlatmapState(context.TODO(), version, m, res, p.Meta())
	if err != nil {
		return nil, err
	}

	// Next, migrate the JSON state up to the current version.
	json, err = resource.UpgradeJSONState(context.TODO(), version, json, res, p.Meta())
	if err != nil {
		return nil, err
	}

	configBlock := res.CoreConfigSchema()

	// Strip out removed fields.
	resource.RemoveAttributes(json, configBlock.ImpliedType())

	// now we need to turn the state into the default json representation, so
	// that it can be re-decoded using the actual schema.
	v, err := schema.JSONMapToStateValue(json, configBlock)
	if err != nil {
		return nil, err
	}

	// Now we need to make sure blocks are represented correctly, which means
	// that missing blocks are empty collections, rather than null.
	// First we need to CoerceValue to ensure that all object types match.
	v, err = configBlock.CoerceValue(v)
	if err != nil {
		return nil, err
	}
	// Normalize the value and fill in any missing blocks.
	v = resource.NormalizeObjectFromLegacySDK(v, configBlock)

	// Convert the value back to an InstanceState.
	newState, err := res.ShimInstanceStateFromValue(v)
	if err != nil {
		return nil, err
	}

	// Copy the original ID and meta to the new state and stamp in the new version.
	newState.ID = instanceState.ID
	newState.Meta = instanceState.Meta
	if hasVersion || version > 0 {
		if newState.Meta == nil {
			newState.Meta = map[string]interface{}{}
		}
		newState.Meta["schema_version"] = strconv.Itoa(version)
	}
	return newState, nil
}
