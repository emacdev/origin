package install

import (
	"k8s.io/kubernetes/pkg/apimachinery/announced"

	"github.com/openshift/origin/pkg/route/api"
	"github.com/openshift/origin/pkg/route/api/v1"
)

func installApiGroup() {
	if err := announced.NewGroupMetaFactory(
		&announced.GroupMetaFactoryArgs{
			GroupName:                  api.GroupName,
			VersionPreferenceOrder:     []string{v1.SchemeGroupVersion.Version},
			ImportPrefix:               importPrefix,
			AddInternalObjectsToScheme: api.AddToScheme,
		},
		announced.VersionToSchemeFunc{
			v1.SchemeGroupVersion.Version: v1.AddToScheme,
		},
	).Announce().RegisterAndEnable(); err != nil {
		panic(err)
	}
}
