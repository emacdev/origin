package util

import (
	"fmt"
	"strconv"
	"strings"

	kapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/labels"

	"github.com/golang/glog"
	buildapi "github.com/openshift/origin/pkg/build/api"
	buildclient "github.com/openshift/origin/pkg/build/client"
)

const (
	// NoBuildLogsMessage reports that no build logs are available
	NoBuildLogsMessage = "No logs are available."
)

// GetBuildName returns name of the build pod.
func GetBuildName(pod *kapi.Pod) string {
	if pod == nil {
		return ""
	}
	return pod.Annotations[buildapi.BuildAnnotation]
}

// GetInputReference returns the From ObjectReference associated with the
// BuildStrategy.
func GetInputReference(strategy buildapi.BuildStrategy) *kapi.ObjectReference {
	switch {
	case strategy.SourceStrategy != nil:
		return &strategy.SourceStrategy.From
	case strategy.DockerStrategy != nil:
		return strategy.DockerStrategy.From
	case strategy.CustomStrategy != nil:
		return &strategy.CustomStrategy.From
	default:
		return nil
	}
}

// IsBuildComplete returns whether the provided build is complete or not
func IsBuildComplete(build *buildapi.Build) bool {
	return build.Status.Phase != buildapi.BuildPhaseRunning && build.Status.Phase != buildapi.BuildPhasePending && build.Status.Phase != buildapi.BuildPhaseNew
}

// IsPaused returns true if the provided BuildConfig is paused and cannot be used to create a new Build
func IsPaused(bc *buildapi.BuildConfig) bool {
	return strings.ToLower(bc.Annotations[buildapi.BuildConfigPausedAnnotation]) == "true"
}

// BuildNumber returns the given build number.
func BuildNumber(build *buildapi.Build) (int64, error) {
	annotations := build.GetAnnotations()
	if stringNumber, ok := annotations[buildapi.BuildNumberAnnotation]; ok {
		return strconv.ParseInt(stringNumber, 10, 64)
	}
	return 0, fmt.Errorf("build %s/%s does not have %s annotation", build.Namespace, build.Name, buildapi.BuildNumberAnnotation)
}

// BuildRunPolicy returns the scheduling policy for the build based on the
// "queued" label.
func BuildRunPolicy(build *buildapi.Build) buildapi.BuildRunPolicy {
	labels := build.GetLabels()
	if value, found := labels[buildapi.BuildRunPolicyLabel]; found {
		switch value {
		case "Parallel":
			return buildapi.BuildRunPolicyParallel
		case "Serial":
			return buildapi.BuildRunPolicySerial
		case "SerialLatestOnly":
			return buildapi.BuildRunPolicySerialLatestOnly
		}
	}
	glog.V(5).Infof("Build %s/%s does not have start policy label set, using default (Serial)", build.Namespace, build.Name)
	return buildapi.BuildRunPolicySerial
}

// BuildNameForConfigVersion returns the name of the version-th build
// for the config that has the provided name.
func BuildNameForConfigVersion(name string, version int) string {
	return fmt.Sprintf("%s-%d", name, version)
}

// BuildConfigSelector returns a label Selector which can be used to find all
// builds for a BuildConfig.
func BuildConfigSelector(name string) labels.Selector {
	return labels.Set{buildapi.BuildConfigLabel: buildapi.LabelValue(name)}.AsSelector()
}

// BuildConfigSelectorDeprecated returns a label Selector which can be used to find
// all builds for a BuildConfig that use the deprecated labels.
func BuildConfigSelectorDeprecated(name string) labels.Selector {
	return labels.Set{buildapi.BuildConfigLabelDeprecated: name}.AsSelector()
}

type buildFilter func(buildapi.Build) bool

// BuildConfigBuilds return a list of builds for the given build config.
// Optionally you can specify a filter function to select only builds that
// matches your criteria.
func BuildConfigBuilds(c buildclient.BuildLister, namespace, name string, filterFunc buildFilter) (*buildapi.BuildList, error) {
	result, err := c.List(namespace, kapi.ListOptions{
		LabelSelector: BuildConfigSelector(name),
	})
	if err != nil {
		return nil, err
	}
	if filterFunc == nil {
		return result, nil
	}
	filteredList := &buildapi.BuildList{TypeMeta: result.TypeMeta, ListMeta: result.ListMeta}
	for _, b := range result.Items {
		if filterFunc(b) {
			filteredList.Items = append(filteredList.Items, b)
		}
	}
	return filteredList, nil
}

// ConfigNameForBuild returns the name of the build config from a
// build name.
func ConfigNameForBuild(build *buildapi.Build) string {
	if build == nil {
		return ""
	}
	if build.Annotations != nil {
		if _, exists := build.Annotations[buildapi.BuildConfigAnnotation]; exists {
			return build.Annotations[buildapi.BuildConfigAnnotation]
		}
	}
	if _, exists := build.Labels[buildapi.BuildConfigLabel]; exists {
		return build.Labels[buildapi.BuildConfigLabel]
	}
	return build.Labels[buildapi.BuildConfigLabelDeprecated]
}

// VersionForBuild returns the version from the provided build name.
// If no version can be found, 0 is returned to indicate no version.
func VersionForBuild(build *buildapi.Build) int {
	if build == nil {
		return 0
	}
	versionString := build.Annotations[buildapi.BuildNumberAnnotation]
	version, err := strconv.Atoi(versionString)
	if err != nil {
		return 0
	}
	return version
}

func BuildDeepCopy(build *buildapi.Build) (*buildapi.Build, error) {
	objCopy, err := kapi.Scheme.DeepCopy(build)
	if err != nil {
		return nil, err
	}
	copied, ok := objCopy.(*buildapi.Build)
	if !ok {
		return nil, fmt.Errorf("expected Build, got %#v", objCopy)
	}
	return copied, nil
}

// MergeTrustedEnvWithoutDuplicates merges two environment lists without having
// duplicate items in the output list.  The source list will be filtered
// such that only whitelisted environment variables are merged into the
// output list.  If sourcePrecedence is true, keys in the source list
// will override keys in the output list.
func MergeTrustedEnvWithoutDuplicates(source []kapi.EnvVar, output *[]kapi.EnvVar, sourcePrecedence bool) {
	// filter out all environment variables except trusted/well known
	// values, because we do not want random environment variables being
	// fed into the privileged STI container via the BuildConfig definition.
	type sourceMapItem struct {
		index int
		value string
	}

	index := 0
	filteredSourceMap := make(map[string]sourceMapItem)
	filteredSource := []kapi.EnvVar{}
	for _, env := range source {
		for _, acceptable := range buildapi.WhitelistEnvVarNames {
			if env.Name == acceptable {
				filteredSource = append(filteredSource, env)
				filteredSourceMap[env.Name] = sourceMapItem{index, env.Value}
				index++
				break
			}
		}
	}

	result := *output
	for i, env := range result {
		// If the value exists in output, override it and remove it
		// from the source list
		if v, found := filteredSourceMap[env.Name]; found {
			if sourcePrecedence {
				result[i].Value = v.value
			}
			filteredSource = append(filteredSource[:v.index], filteredSource[v.index+1:]...)
		}
	}
	*output = append(result, filteredSource...)
}
