package ps

import (
	"context"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"

	"terraform-percona/internal/cloud"
	"terraform-percona/internal/cloud/aws"
	"terraform-percona/internal/resource"
	"terraform-percona/internal/utils"
)

const (
	ReplicaPassword = "replica_password"
	MyRocksInstall  = "myrocks_install"
)

func Resource() *schema.Resource {
	return &schema.Resource{
		CreateContext: createResource,
		ReadContext:   readResource,
		UpdateContext: updateResource,
		DeleteContext: deleteResource,

		Schema: utils.MergeSchemas(resource.DefaultSchema(), aws.Schema(), map[string]*schema.Schema{
			ReplicaPassword: {
				Type:      schema.TypeString,
				Optional:  true,
				Default:   "replicaPassword",
				Sensitive: true,
			},
			MyRocksInstall: {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			resource.Instances: {
				Type:     schema.TypeSet,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"public_ip_address": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"private_ip_address": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"is_replica": {
							Type:     schema.TypeBool,
							Computed: true,
						},
					},
				},
			},
		}),
	}
}

func createResource(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c, ok := meta.(cloud.Cloud)
	if !ok {
		return diag.Errorf("failed to get cloud controller")
	}

	resourceID := utils.GetRandomString(resource.IDLength)

	err := c.Configure(ctx, resourceID, data)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't configure cloud"))
	}

	data.SetId(resourceID)
	err = c.CreateInfrastructure(ctx, resourceID)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't create cloud infrastructure"))
	}

	manager := newManager(c, resourceID, data)
	instances, err := manager.createCluster(ctx)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't create ps cluster"))
	}

	set := data.Get(resource.Instances).(*schema.Set)
	for i, instance := range instances {
		set.Add(map[string]interface{}{
			"is_replica":         i == 0,
			"public_ip_address":  instance.PublicIpAddress,
			"private_ip_address": instance.PrivateIpAddress,
		})
	}
	err = data.Set(resource.Instances, set)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't set instances"))
	}

	args := make(map[string]interface{})
	if manager.size > 1 {
		for i, instance := range instances {
			if i == 0 {
				args[resource.LogArgMasterIP] = instance.PublicIpAddress
				continue
			}
			if args[resource.LogArgReplicaIP] == nil {
				args[resource.LogArgReplicaIP] = []interface{}{}
			}
			args[resource.LogArgReplicaIP] = append(args[resource.LogArgReplicaIP].([]interface{}), instance.PublicIpAddress)
		}
	} else if manager.size == 1 {
		args[resource.LogArgMasterIP] = instances[0].PublicIpAddress
	}
	tflog.Info(ctx, "Percona Server resource created", args)
	return nil
}

func readResource(_ context.Context, _ *schema.ResourceData, _ interface{}) diag.Diagnostics {
	//TODO
	return nil
}

func updateResource(_ context.Context, _ *schema.ResourceData, _ interface{}) diag.Diagnostics {
	//TODO
	return nil
}

func deleteResource(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c, ok := meta.(cloud.Cloud)
	if !ok {
		return diag.Errorf("failed to get cloud controller")
	}

	resourceID := data.Id()
	if resourceID == "" {
		return diag.Errorf("empty resource id")
	}

	err := c.Configure(ctx, resourceID, data)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't configure cloud"))
	}

	err = c.DeleteInfrastructure(ctx, resourceID)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't delete cloud infrastructure"))
	}
	return nil
}
