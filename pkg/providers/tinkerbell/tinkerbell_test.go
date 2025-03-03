package tinkerbell

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	tinkv1alpha1 "github.com/tinkerbell/tink/pkg/apis/core/v1alpha1"

	"github.com/aws/eks-anywhere/internal/test"
	"github.com/aws/eks-anywhere/pkg/api/v1alpha1"
	"github.com/aws/eks-anywhere/pkg/cluster"
	"github.com/aws/eks-anywhere/pkg/constants"
	"github.com/aws/eks-anywhere/pkg/filewriter"
	filewritermocks "github.com/aws/eks-anywhere/pkg/filewriter/mocks"
	"github.com/aws/eks-anywhere/pkg/providers/tinkerbell/mocks"
	"github.com/aws/eks-anywhere/pkg/providers/tinkerbell/stack"
	stackmocks "github.com/aws/eks-anywhere/pkg/providers/tinkerbell/stack/mocks"
	"github.com/aws/eks-anywhere/pkg/types"
	"github.com/aws/eks-anywhere/pkg/utils/ptr"
)

const (
	testDataDir = "testdata"
	testIP      = "5.6.7.8"
)

func givenClusterSpec(t *testing.T, fileName string) *cluster.Spec {
	return test.NewFullClusterSpec(t, path.Join(testDataDir, fileName))
}

func givenDatacenterConfig(t *testing.T, fileName string) *v1alpha1.TinkerbellDatacenterConfig {
	datacenterConfig, err := v1alpha1.GetTinkerbellDatacenterConfig(path.Join(testDataDir, fileName))
	if err != nil {
		t.Fatalf("unable to get datacenter config from file: %v", err)
	}
	return datacenterConfig
}

func givenMachineConfigs(t *testing.T, fileName string) map[string]*v1alpha1.TinkerbellMachineConfig {
	machineConfigs, err := v1alpha1.GetTinkerbellMachineConfigs(path.Join(testDataDir, fileName))
	if err != nil {
		t.Fatalf("unable to get machine configs from file: %v", err)
	}
	return machineConfigs
}

func assertError(t *testing.T, expected string, err error) {
	if err == nil {
		t.Fatalf("Expected=<%s> actual=<nil>", expected)
	}
	actual := err.Error()
	if expected != actual {
		t.Fatalf("Expected=<%s> actual=<%s>", expected, actual)
	}
}

func newProvider(datacenterConfig *v1alpha1.TinkerbellDatacenterConfig, machineConfigs map[string]*v1alpha1.TinkerbellMachineConfig, clusterConfig *v1alpha1.Cluster, writer filewriter.FileWriter, docker stack.Docker, helm stack.Helm, kubectl ProviderKubectlClient, forceCleanup bool) *Provider {
	hardwareFile := "./testdata/hardware.csv"
	provider, err := NewProvider(
		datacenterConfig,
		machineConfigs,
		clusterConfig,
		hardwareFile,
		writer,
		docker,
		helm,
		kubectl,
		testIP,
		test.FakeNow,
		forceCleanup,
		false,
	)
	if err != nil {
		panic(err)
	}

	return provider
}

func TestTinkerbellProviderGenerateDeploymentFileWithExternalEtcd(t *testing.T) {
	t.Skip("External etcd unsupported for GA")
	clusterSpecManifest := "cluster_tinkerbell_external_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_cluster_tinkerbell_cp_external_etcd.yaml")
	test.AssertContentToFile(t, string(md), "testdata/expected_results_cluster_tinkerbell_md.yaml")
}

func TestTinkerbellProviderWithExternalEtcdFail(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_external_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec)
	assert.Error(t, err, "expect validation to fail because external etcd is not supported")

	err = provider.SetupAndValidateUpgradeCluster(ctx, cluster, clusterSpec, clusterSpec)
	assert.Error(t, err, "expect validation to fail because external etcd is not supported")
}

func TestTinkerbellProviderMachineConfigsMissingUserSshKeys(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_missing_ssh_keys.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	keyGenerator := mocks.NewMockSSHAuthKeyGenerator(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	const sshKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQC1BK73XhIzjX+meUr7pIYh6RHbvI3tmHeQIXY5lv7aztN1UoX+bhPo3dwo2sfSQn5kuxgQdnxIZ/CTzy0p0GkEYVv3gwspCeurjmu0XmrdmaSGcGxCEWT/65NtvYrQtUE5ELxJ+N/aeZNlK2B7IWANnw/82913asXH4VksV1NYNduP0o1/G4XcwLLSyVFB078q/oEnmvdNIoS61j4/o36HVtENJgYr0idcBvwJdvcGxGnPaqOhx477t+kfJAa5n5dSA5wilIaoXH5i1Tf/HsTCM52L+iNCARvQzJYZhzbWI1MDQwzILtIBEQCJsl2XSqIupleY8CxqQ6jCXt2mhae+wPc3YmbO5rFvr2/EvC57kh3yDs1Nsuj8KOvD78KeeujbR8n8pScm3WDp62HFQ8lEKNdeRNj6kB8WnuaJvPnyZfvzOhwG65/9w13IBl7B1sWxbFnq2rMpm5uHVK7mAmjL0Tt8zoDhcE1YJEnp9xte3/pvmKPkST5Q/9ZtR9P5sI+02jY0fvPkPyC03j2gsPixG7rpOCwpOdbny4dcj0TDeeXJX8er+oVfJuLYz0pNWJcT2raDdFfcqvYA0B0IyNYlj5nWX4RuEcyT3qocLReWPnZojetvAG/H8XwOh7fEVGqHAKOVSnPXCSQJPl6s0H12jPJBDJMTydtYPEszl4/CeQ=="
	keyGenerator.EXPECT().GenerateSSHAuthKey(gomock.Any()).Return(sshKey, nil)

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)

	// Hack: monkey patch the key generator and the stack installer directly for determinism.
	provider.keyGenerator = keyGenerator
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, _, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_cluster_tinkerbell_missing_ssh_keys.yaml")
}

func TestTinkerbellProviderGenerateDeploymentFileWithStackedEtcd(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_cluster_tinkerbell_cp_stacked_etcd.yaml")
	test.AssertContentToFile(t, string(md), "testdata/expected_results_cluster_tinkerbell_md.yaml")
}

func TestTinkerbellProviderGenerateDeploymentFileWithAutoscalerConfiguration(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	wng := &clusterSpec.Cluster.Spec.WorkerNodeGroupConfigurations[0]
	ca := &v1alpha1.AutoScalingConfiguration{
		MaxCount: 5,
		MinCount: 3,
	}
	wng.AutoScalingConfiguration = ca
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_cluster_tinkerbell_cp_stacked_etcd.yaml")
	test.AssertContentToFile(t, string(md), "testdata/expected_results_cluster_tinkerbell_autoscaler_md.yaml")
}

func TestTinkerbellProviderGenerateDeploymentFileWithNodeLabels(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_node_labels.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_cluster_tinkerbell_cp_node_labels.yaml")
	test.AssertContentToFile(t, string(md), "testdata/expected_results_cluster_tinkerbell_md_node_labels.yaml")
}

func TestTinkerbellProviderGenerateDeploymentFileWithNodeTaints(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_node_taints.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_cluster_tinkerbell_cp_node_taints.yaml")
	test.AssertContentToFile(t, string(md), "testdata/expected_results_cluster_tinkerbell_md_node_taints.yaml")
}

func TestTinkerbellProviderGenerateDeploymentFileMultipleWorkerNodeGroups(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_multiple_node_groups.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_cluster_tinkerbell_cp_external_etcd.yaml")
	test.AssertContentToFile(t, string(md), "testdata/expected_results_tinkerbell_md_multiple_node_groups.yaml")
}

func TestPreCAPIInstallOnBootstrapSuccess(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test", KubeconfigFile: "test.kubeconfig"}
	ctx := context.Background()
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().Install(
		ctx,
		clusterSpec.VersionsBundle.Tinkerbell,
		testIP,
		"test.kubeconfig",
		"",
		gomock.Any(),
		gomock.Any(),
	)

	err := provider.PreCAPIInstallOnBootstrap(ctx, cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed PreCAPIInstallOnBootstrap: %v", err)
	}
}

func TestPostWorkloadInitSuccess(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test", KubeconfigFile: "test.kubeconfig"}
	ctx := context.Background()
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().Install(
		ctx,
		clusterSpec.VersionsBundle.Tinkerbell,
		testIP,
		"test.kubeconfig",
		"",
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	)
	stackInstaller.EXPECT().UninstallLocal(ctx)

	err := provider.PostWorkloadInit(ctx, cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed PostWorkloadInit: %v", err)
	}
}

func TestPostBootstrapSetupSuccess(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test", KubeconfigFile: "test.kubeconfig"}
	ctx := context.Background()
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)

	kubectl.EXPECT().ApplyKubeSpecFromBytesForce(ctx, cluster, gomock.Any())
	kubectl.EXPECT().WaitForRufioMachines(ctx, cluster, "5m", "Contactable", gomock.Any()).MaxTimes(2)

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	if err := provider.readCSVToCatalogue(); err != nil {
		t.Fatalf("failed to read hardware csv: %v", err)
	}

	err := provider.PostBootstrapSetup(ctx, provider.clusterConfig, cluster)
	if err != nil {
		t.Fatalf("failed PostBootstrapSetup: %v", err)
	}
}

func TestPostBootstrapSetupWaitForRufioMachinesFail(t *testing.T) {
	wantError := errors.New("test error")
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test", KubeconfigFile: "test.kubeconfig"}
	ctx := context.Background()
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)

	kubectl.EXPECT().ApplyKubeSpecFromBytesForce(ctx, cluster, gomock.Any())
	kubectl.EXPECT().WaitForRufioMachines(ctx, cluster, "5m", "Contactable", gomock.Any()).Return(wantError)

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	if err := provider.readCSVToCatalogue(); err != nil {
		t.Fatalf("failed to read hardware csv: %v", err)
	}

	err := provider.PostBootstrapSetup(ctx, provider.clusterConfig, cluster)
	assert.Error(t, err, "PostBootstrapSetup should fail")
}

func TestPostMoveManagementToBootstrapSuccess(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test", KubeconfigFile: "test.kubeconfig"}
	ctx := context.Background()
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)

	kubectl.EXPECT().WaitForRufioMachines(ctx, cluster, "5m", "Contactable", gomock.Any()).Return(nil).MaxTimes(2)

	tt := []struct {
		name            string
		hardwareCSVFile string
	}{
		{
			name:            "bmc in hardware csv",
			hardwareCSVFile: "./testdata/hardware.csv",
		},
		{
			name:            "no bmc in hardware csv",
			hardwareCSVFile: "./testdata/hardware_nobmc.csv",
		},
	}

	for _, test := range tt {
		t.Run(test.name, func(t *testing.T) {
			provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
			provider.hardwareCSVFile = test.hardwareCSVFile
			if err := provider.readCSVToCatalogue(); err != nil {
				t.Fatalf("failed to read hardware csv: %v", err)
			}

			err := provider.PostMoveManagementToBootstrap(ctx, cluster)
			if err != nil {
				t.Fatalf("failed PostMoveManagementToBootstrap: %v", err)
			}
		})
	}
}

func TestPostMoveManagementToBootstrapWaitForRufioMachinesFail(t *testing.T) {
	wantError := errors.New("test error")
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test", KubeconfigFile: "test.kubeconfig"}
	ctx := context.Background()
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)

	kubectl.EXPECT().WaitForRufioMachines(ctx, cluster, "5m", "Contactable", gomock.Any()).Return(wantError)
	if err := provider.readCSVToCatalogue(); err != nil {
		t.Fatalf("failed to read hardware csv: %v", err)
	}

	err := provider.PostMoveManagementToBootstrap(ctx, cluster)
	assert.Error(t, err, "PostMoveManagementToBootstrap should fail")
}

func TestTinkerbellProviderGenerateDeploymentFileWithFullOIDC(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_full_oidc.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_cluster_tinkerbell_cp_full_oidc.yaml")
	test.AssertContentToFile(t, string(md), "testdata/expected_results_cluster_tinkerbell_md.yaml")
}

func TestTinkerbellProviderGenerateDeploymentFileWithMinimalOIDC(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_minimal_oidc.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_cluster_tinkerbell_cp_minimal_oidc.yaml")
	test.AssertContentToFile(t, string(md), "testdata/expected_results_cluster_tinkerbell_md.yaml")
}

func TestTinkerbellProviderGenerateDeploymentFileWithAWSIamConfig(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_awsiam.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_cluster_tinkerbell_cp_awsiam.yaml")
	test.AssertContentToFile(t, string(md), "testdata/expected_results_cluster_tinkerbell_md.yaml")
}

func TestProviderGenerateDeploymentFileForWithMinimalRegistryMirror(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_minimal_registry_mirror.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_cluster_tinkerbell_cp_minimal_registry_mirror.yaml")
	test.AssertContentToFile(t, string(md), "testdata/expected_results_cluster_tinkerbell_md_minimal_registry_mirror.yaml")
}

func TestProviderGenerateDeploymentFileForWithRegistryMirrorWithCert(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_registry_mirror_with_cert.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_cluster_tinkerbell_cp_registry_mirror_with_cert.yaml")
	test.AssertContentToFile(t, string(md), "testdata/expected_results_cluster_tinkerbell_md_registry_mirror_with_cert.yaml")
}

func TestProviderGenerateDeploymentFileForWithRegistryMirrorWithAuth(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_registry_mirror_with_auth.yaml"
	if err := os.Setenv("REGISTRY_USERNAME", "username"); err != nil {
		t.Fatalf(err.Error())
	}
	if err := os.Setenv("REGISTRY_PASSWORD", "password"); err != nil {
		t.Fatalf(err.Error())
	}
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_cluster_tinkerbell_cp_registry_mirror_with_auth.yaml")
	test.AssertContentToFile(t, string(md), "testdata/expected_results_cluster_tinkerbell_md_registry_mirror_with_auth.yaml")
}

func TestProviderGenerateDeploymentFileForWithBottlerocketMinimalRegistryMirror(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_bottlerocket_minimal_registry_mirror.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_cluster_tinkerbell_bottlerocket_cp_minimal_registry_mirror.yaml")
	test.AssertContentToFile(t, string(md), "testdata/expected_results_cluster_tinkerbell_bottlerocket_md_minimal_registry_mirror.yaml")
}

func TestProviderGenerateDeploymentFileForWithBottlerocketRegistryMirrorWithCert(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_bottlerocket_registry_mirror_with_cert.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_cluster_tinkerbell_bottlerocket_cp_registry_mirror_with_cert.yaml")
	test.AssertContentToFile(t, string(md), "testdata/expected_results_cluster_tinkerbell_bottlerocket_md_registry_mirror_with_cert.yaml")
}

func TestProviderGenerateDeploymentFileForWithBottlerocketRegistryMirrorWithAuth(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_bottlerocket_registry_mirror_with_auth.yaml"
	t.Setenv("REGISTRY_USERNAME", "username")
	t.Setenv("REGISTRY_PASSWORD", "password")

	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_cluster_tinkerbell_bottlerocket_cp_registry_mirror_with_auth.yaml")
	test.AssertContentToFile(t, string(md), "testdata/expected_results_cluster_tinkerbell_bottlerocket_md_registry_mirror_with_auth.yaml")
}

func TestProviderGenerateDeploymentFileForSingleNodeCluster(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_single_node.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	if len(md) != 0 {
		t.Fatalf("expect nothing to be generated for worker node")
	}
	test.AssertContentToFile(t, string(cp), "testdata/expected_results_cluster_tinkerbell_cp_single_node.yaml")
}

func TestProviderGenerateDeploymentFileForSingleNodeClusterSkipLB(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_single_node_skip_lb.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	if len(md) != 0 {
		t.Fatalf("expect nothing to be generated for worker node")
	}
	test.AssertContentToFile(t, string(cp), "testdata/expected_results_cluster_tinkerbell_cp_single_node_skip_lb.yaml")
}

func TestTinkerbellTemplate_isScaleUpDownSuccess(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)

	newClusterSpec := clusterSpec.DeepCopy()
	newClusterSpec.Cluster.Spec.WorkerNodeGroupConfigurations[0].Count = ptr.Int(2)

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	assert.True(t, provider.isScaleUpDown(clusterSpec.Cluster, newClusterSpec.Cluster), "expected scale up down true")
}

func TestSetupAndValidateCreateWorkloadClusterSuccess(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)

	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)
	provider.providerKubectlClient = kubectl

	clusterSpec.Cluster.SetManagedBy("management-cluster")
	clusterSpec.ManagementCluster = &types.Cluster{
		Name:               "management-cluster",
		KubeconfigFile:     "kc.kubeconfig",
		ExistingManagement: true,
	}
	for _, config := range machineConfigs {
		kubectl.EXPECT().SearchTinkerbellMachineConfig(ctx, config.Name, clusterSpec.ManagementCluster.KubeconfigFile, config.Namespace).Return([]*v1alpha1.TinkerbellMachineConfig{}, nil)
	}
	kubectl.EXPECT().SearchTinkerbellDatacenterConfig(ctx, datacenterConfig.Name, clusterSpec.ManagementCluster.KubeconfigFile, clusterSpec.Cluster.Namespace).Return([]*v1alpha1.TinkerbellDatacenterConfig{}, nil)

	kubectl.EXPECT().GetUnprovisionedTinkerbellHardware(ctx, clusterSpec.ManagementCluster.KubeconfigFile, constants.EksaSystemNamespace).Return([]tinkv1alpha1.Hardware{}, nil)
	kubectl.EXPECT().GetProvisionedTinkerbellHardware(ctx, clusterSpec.ManagementCluster.KubeconfigFile, constants.EksaSystemNamespace).Return([]tinkv1alpha1.Hardware{}, nil)
	kubectl.EXPECT().GetEksaCluster(ctx, clusterSpec.ManagementCluster, clusterSpec.ManagementCluster.Name).Return(clusterSpec.Cluster, nil)
	kubectl.EXPECT().GetEksaTinkerbellDatacenterConfig(ctx, datacenterConfig.Name, clusterSpec.ManagementCluster.KubeconfigFile, clusterSpec.Cluster.Namespace).Return(datacenterConfig, nil)
	kubectl.EXPECT().ApplyKubeSpecFromBytesForce(ctx, clusterSpec.ManagementCluster, gomock.Any())
	kubectl.EXPECT().WaitForRufioMachines(ctx, clusterSpec.ManagementCluster, "5m", "Contactable", constants.EksaSystemNamespace)

	err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec)
	if err != nil {
		t.Fatalf("unexpected failure %v", err)
	}
	assert.NoError(t, err, "No error should be returned")
}

func TestSetupAndValidateCreateWorkloadClusterFailsIfMachineExists(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)

	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)
	provider.providerKubectlClient = kubectl

	clusterSpec.Cluster.SetManagedBy("management-cluster")
	clusterSpec.ManagementCluster = &types.Cluster{
		Name:               "management-cluster",
		KubeconfigFile:     "kc.kubeconfig",
		ExistingManagement: true,
	}

	idx := 0
	var existingMachine string
	for _, config := range machineConfigs {
		if idx == 0 {
			kubectl.EXPECT().SearchTinkerbellMachineConfig(ctx, config.Name, clusterSpec.ManagementCluster.KubeconfigFile, config.Namespace).Return([]*v1alpha1.TinkerbellMachineConfig{config}, nil)
			existingMachine = config.Name
		} else {
			kubectl.EXPECT().SearchTinkerbellMachineConfig(ctx, config.Name, clusterSpec.ManagementCluster.KubeconfigFile, config.Namespace).Return([]*v1alpha1.TinkerbellMachineConfig{}, nil).MaxTimes(1)
		}
		idx++
	}

	err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec)

	assertError(t, fmt.Sprintf("TinkerbellMachineConfig %s already exists", existingMachine), err)
}

func TestSetupAndValidateCreateWorkloadClusterFailsIfDatacenterExists(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)

	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)
	provider.providerKubectlClient = kubectl

	clusterSpec.Cluster.SetManagedBy("management-cluster")
	clusterSpec.ManagementCluster = &types.Cluster{
		Name:               "management-cluster",
		KubeconfigFile:     "kc.kubeconfig",
		ExistingManagement: true,
	}

	for _, config := range machineConfigs {
		kubectl.EXPECT().SearchTinkerbellMachineConfig(ctx, config.Name, clusterSpec.ManagementCluster.KubeconfigFile, config.Namespace).Return([]*v1alpha1.TinkerbellMachineConfig{}, nil)
	}
	kubectl.EXPECT().SearchTinkerbellDatacenterConfig(ctx, datacenterConfig.Name, clusterSpec.ManagementCluster.KubeconfigFile, clusterSpec.Cluster.Namespace).Return([]*v1alpha1.TinkerbellDatacenterConfig{datacenterConfig}, nil)

	err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec)

	assertError(t, fmt.Sprintf("TinkerbellDatacenterConfig %s already exists", datacenterConfig.Name), err)
}

func TestSetupAndValidateCreateWorkloadClusterFailsIfDatacenterConfigError(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)

	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)
	provider.providerKubectlClient = kubectl

	clusterSpec.Cluster.SetManagedBy("management-cluster")
	clusterSpec.ManagementCluster = &types.Cluster{
		Name:               "management-cluster",
		KubeconfigFile:     "kc.kubeconfig",
		ExistingManagement: true,
	}

	for _, config := range machineConfigs {
		kubectl.EXPECT().SearchTinkerbellMachineConfig(ctx, config.Name, clusterSpec.ManagementCluster.KubeconfigFile, config.Namespace).Return([]*v1alpha1.TinkerbellMachineConfig{}, nil)
	}
	kubectl.EXPECT().SearchTinkerbellDatacenterConfig(ctx, datacenterConfig.Name, clusterSpec.ManagementCluster.KubeconfigFile, clusterSpec.Cluster.Namespace).Return([]*v1alpha1.TinkerbellDatacenterConfig{}, errors.New("error getting TinkerbellDatacenterConfig"))

	err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec)
	assertError(t, "error getting TinkerbellDatacenterConfig", err)
}

func TestSetupAndValidateCreateWorkloadClusterErrorUnprovisionedHardware(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)

	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)
	provider.providerKubectlClient = kubectl

	clusterSpec.Cluster.SetManagedBy("management-cluster")
	clusterSpec.ManagementCluster = &types.Cluster{
		Name:               "management-cluster",
		KubeconfigFile:     "kc.kubeconfig",
		ExistingManagement: true,
	}
	for _, config := range machineConfigs {
		kubectl.EXPECT().SearchTinkerbellMachineConfig(ctx, config.Name, clusterSpec.ManagementCluster.KubeconfigFile, config.Namespace).Return([]*v1alpha1.TinkerbellMachineConfig{}, nil)
	}
	kubectl.EXPECT().SearchTinkerbellDatacenterConfig(ctx, datacenterConfig.Name, clusterSpec.ManagementCluster.KubeconfigFile, clusterSpec.Cluster.Namespace).Return([]*v1alpha1.TinkerbellDatacenterConfig{}, nil)

	kubectl.EXPECT().GetUnprovisionedTinkerbellHardware(ctx, clusterSpec.ManagementCluster.KubeconfigFile, constants.EksaSystemNamespace).Return([]tinkv1alpha1.Hardware{}, errors.New("error"))

	err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec)
	assertError(t, "retrieving unprovisioned hardware: error", err)
}

func TestSetupAndValidateCreateWorkloadClusterErrorProvisionedHardware(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)

	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)
	provider.providerKubectlClient = kubectl

	clusterSpec.Cluster.SetManagedBy("management-cluster")
	clusterSpec.ManagementCluster = &types.Cluster{
		Name:               "management-cluster",
		KubeconfigFile:     "kc.kubeconfig",
		ExistingManagement: true,
	}
	for _, config := range machineConfigs {
		kubectl.EXPECT().SearchTinkerbellMachineConfig(ctx, config.Name, clusterSpec.ManagementCluster.KubeconfigFile, config.Namespace).Return([]*v1alpha1.TinkerbellMachineConfig{}, nil)
	}
	kubectl.EXPECT().SearchTinkerbellDatacenterConfig(ctx, datacenterConfig.Name, clusterSpec.ManagementCluster.KubeconfigFile, clusterSpec.Cluster.Namespace).Return([]*v1alpha1.TinkerbellDatacenterConfig{}, nil)

	kubectl.EXPECT().GetUnprovisionedTinkerbellHardware(ctx, clusterSpec.ManagementCluster.KubeconfigFile, constants.EksaSystemNamespace).Return([]tinkv1alpha1.Hardware{}, nil)

	kubectl.EXPECT().GetProvisionedTinkerbellHardware(ctx, clusterSpec.ManagementCluster.KubeconfigFile, constants.EksaSystemNamespace).Return([]tinkv1alpha1.Hardware{}, errors.New("error"))

	err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec)
	assertError(t, "retrieving provisioned hardware: error", err)
}

func TestSetupAndValidateUpgradeClusterErrorValidateClusterSpec(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller
	provider.providerKubectlClient = kubectl

	cluster.KubeconfigFile = "kc.kubeconfig"

	kubectl.EXPECT().GetUnprovisionedTinkerbellHardware(ctx, cluster.KubeconfigFile, constants.EksaSystemNamespace).Return([]tinkv1alpha1.Hardware{}, nil)

	kubectl.EXPECT().GetProvisionedTinkerbellHardware(ctx, cluster.KubeconfigFile, constants.EksaSystemNamespace).Return([]tinkv1alpha1.Hardware{}, nil)

	provider.datacenterConfig.Spec.TinkerbellIP = ""

	err := provider.SetupAndValidateUpgradeCluster(ctx, cluster, clusterSpec, clusterSpec)
	assertError(t, "TinkerbellDatacenterConfig: missing spec.tinkerbellIP field", err)
}

func TestSetupAndValidateUpgradeWorkloadClusterErrorApplyHardware(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller
	provider.providerKubectlClient = kubectl

	clusterSpec.Cluster.SetManagedBy("management-cluster")
	clusterSpec.ManagementCluster = &types.Cluster{
		Name:               "management-cluster",
		KubeconfigFile:     "kc.kubeconfig",
		ExistingManagement: true,
	}
	cluster.KubeconfigFile = "kc.kubeconfig"

	kubectl.EXPECT().GetUnprovisionedTinkerbellHardware(ctx, clusterSpec.ManagementCluster.KubeconfigFile, constants.EksaSystemNamespace).Return([]tinkv1alpha1.Hardware{}, nil)

	kubectl.EXPECT().GetProvisionedTinkerbellHardware(ctx, clusterSpec.ManagementCluster.KubeconfigFile, constants.EksaSystemNamespace).Return([]tinkv1alpha1.Hardware{}, nil)

	kubectl.EXPECT().ApplyKubeSpecFromBytesForce(ctx, clusterSpec.ManagementCluster, gomock.Any()).Return(fmt.Errorf("error"))

	err := provider.SetupAndValidateUpgradeCluster(ctx, cluster, clusterSpec, clusterSpec)
	assertError(t, "applying hardware yaml: error", err)
}

func TestSetupAndValidateUpgradeWorkloadClusterErrorBMC(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller
	provider.providerKubectlClient = kubectl
	provider.hardwareCSVFile = "testdata/hardware.csv"

	clusterSpec.Cluster.SetManagedBy("management-cluster")
	clusterSpec.ManagementCluster = &types.Cluster{
		Name:               "management-cluster",
		KubeconfigFile:     "kc.kubeconfig",
		ExistingManagement: true,
	}
	cluster.KubeconfigFile = "kc.kubeconfig"

	kubectl.EXPECT().GetUnprovisionedTinkerbellHardware(ctx, clusterSpec.ManagementCluster.KubeconfigFile, constants.EksaSystemNamespace).Return([]tinkv1alpha1.Hardware{}, nil)

	kubectl.EXPECT().GetProvisionedTinkerbellHardware(ctx, clusterSpec.ManagementCluster.KubeconfigFile, constants.EksaSystemNamespace).Return([]tinkv1alpha1.Hardware{}, nil)

	kubectl.EXPECT().ApplyKubeSpecFromBytesForce(ctx, clusterSpec.ManagementCluster, gomock.Any())

	kubectl.EXPECT().WaitForRufioMachines(ctx, cluster, "5m", "Contactable", gomock.Any()).Return(fmt.Errorf("error"))

	err := provider.SetupAndValidateUpgradeCluster(ctx, cluster, clusterSpec, clusterSpec)
	assertError(t, "waiting for baseboard management to be contactable: error", err)
}

func TestSetupAndValidateCreateWorkloadClusterErrorManagementCluster(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)

	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)
	provider.providerKubectlClient = kubectl

	clusterSpec.Cluster.SetManagedBy("management-cluster")
	clusterSpec.ManagementCluster = &types.Cluster{
		Name:               "management-cluster",
		KubeconfigFile:     "kc.kubeconfig",
		ExistingManagement: true,
	}
	for _, config := range machineConfigs {
		kubectl.EXPECT().SearchTinkerbellMachineConfig(ctx, config.Name, clusterSpec.ManagementCluster.KubeconfigFile, config.Namespace).Return([]*v1alpha1.TinkerbellMachineConfig{}, nil)
	}
	kubectl.EXPECT().SearchTinkerbellDatacenterConfig(ctx, datacenterConfig.Name, clusterSpec.ManagementCluster.KubeconfigFile, clusterSpec.Cluster.Namespace).Return([]*v1alpha1.TinkerbellDatacenterConfig{}, nil)

	kubectl.EXPECT().GetUnprovisionedTinkerbellHardware(ctx, clusterSpec.ManagementCluster.KubeconfigFile, constants.EksaSystemNamespace).Return([]tinkv1alpha1.Hardware{}, nil)
	kubectl.EXPECT().GetProvisionedTinkerbellHardware(ctx, clusterSpec.ManagementCluster.KubeconfigFile, constants.EksaSystemNamespace).Return([]tinkv1alpha1.Hardware{}, nil)
	kubectl.EXPECT().GetEksaCluster(ctx, clusterSpec.ManagementCluster, clusterSpec.ManagementCluster.Name).Return(clusterSpec.Cluster, fmt.Errorf("error getting management cluster data"))

	err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec)
	assertError(t, "error getting management cluster data", err)
}

func TestSetupAndValidateCreateWorkloadClusterErrorManagementClusterTinkerbellIP(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)

	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)
	provider.providerKubectlClient = kubectl

	clusterSpec.Cluster.SetManagedBy("management-cluster")
	clusterSpec.ManagementCluster = &types.Cluster{
		Name:               "management-cluster",
		KubeconfigFile:     "kc.kubeconfig",
		ExistingManagement: true,
	}
	for _, config := range machineConfigs {
		kubectl.EXPECT().SearchTinkerbellMachineConfig(ctx, config.Name, clusterSpec.ManagementCluster.KubeconfigFile, config.Namespace).Return([]*v1alpha1.TinkerbellMachineConfig{}, nil)
	}
	kubectl.EXPECT().SearchTinkerbellDatacenterConfig(ctx, datacenterConfig.Name, clusterSpec.ManagementCluster.KubeconfigFile, clusterSpec.Cluster.Namespace).Return([]*v1alpha1.TinkerbellDatacenterConfig{}, nil)

	kubectl.EXPECT().GetUnprovisionedTinkerbellHardware(ctx, clusterSpec.ManagementCluster.KubeconfigFile, constants.EksaSystemNamespace).Return([]tinkv1alpha1.Hardware{}, nil)
	kubectl.EXPECT().GetProvisionedTinkerbellHardware(ctx, clusterSpec.ManagementCluster.KubeconfigFile, constants.EksaSystemNamespace).Return([]tinkv1alpha1.Hardware{}, nil)
	kubectl.EXPECT().GetEksaCluster(ctx, clusterSpec.ManagementCluster, clusterSpec.ManagementCluster.Name).Return(clusterSpec.Cluster, nil)
	kubectl.EXPECT().GetEksaTinkerbellDatacenterConfig(ctx, datacenterConfig.Name, clusterSpec.ManagementCluster.KubeconfigFile, clusterSpec.Cluster.Namespace).Return(datacenterConfig, fmt.Errorf("error"))

	err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec)
	assertError(t, "getting TinkerbellIP of management cluster: error", err)
}

func TestPreCoreComponentsUpgrade(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_stacked_etcd.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)

	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)
	provider.providerKubectlClient = kubectl

	clusterSpec.Cluster.SetManagedBy("management-cluster")
	clusterSpec.ManagementCluster = &types.Cluster{
		Name:               "management-cluster",
		KubeconfigFile:     "kc.kubeconfig",
		ExistingManagement: true,
	}
	for _, config := range machineConfigs {
		kubectl.EXPECT().SearchTinkerbellMachineConfig(ctx, config.Name, clusterSpec.ManagementCluster.KubeconfigFile, config.Namespace).Return([]*v1alpha1.TinkerbellMachineConfig{}, nil)
	}
	kubectl.EXPECT().SearchTinkerbellDatacenterConfig(ctx, datacenterConfig.Name, clusterSpec.ManagementCluster.KubeconfigFile, clusterSpec.Cluster.Namespace).Return([]*v1alpha1.TinkerbellDatacenterConfig{}, nil)

	kubectl.EXPECT().GetUnprovisionedTinkerbellHardware(ctx, clusterSpec.ManagementCluster.KubeconfigFile, constants.EksaSystemNamespace).Return([]tinkv1alpha1.Hardware{}, nil)
	kubectl.EXPECT().GetProvisionedTinkerbellHardware(ctx, clusterSpec.ManagementCluster.KubeconfigFile, constants.EksaSystemNamespace).Return([]tinkv1alpha1.Hardware{}, nil)
	kubectl.EXPECT().GetEksaCluster(ctx, clusterSpec.ManagementCluster, clusterSpec.ManagementCluster.Name).Return(clusterSpec.Cluster, nil)
	kubectl.EXPECT().GetEksaTinkerbellDatacenterConfig(ctx, datacenterConfig.Name, clusterSpec.ManagementCluster.KubeconfigFile, clusterSpec.Cluster.Namespace).Return(datacenterConfig, fmt.Errorf("error"))

	err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec)
	assertError(t, "getting TinkerbellIP of management cluster: error", err)
}

func TestProviderGenerateDeploymentFileForWithProxy(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_proxy.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_cluster_tinkerbell_cp_proxy.yaml")
	test.AssertContentToFile(t, string(md), "testdata/expected_results_cluster_tinkerbell_md_proxy.yaml")
}

func TestProviderGenerateDeploymentFileForBottleRocketWithNTPConfig(t *testing.T) {
	clusterSpecManifest := "cluster_bottlerocket_ntp_config.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_bottlerocket_ntp_config_cp.yaml")
	test.AssertContentToFile(t, string(md), "testdata/expected_results_bottlerocket_ntp_config_md.yaml")
}

func TestProviderGenerateDeploymentFileForUbuntuWithNTPConfig(t *testing.T) {
	clusterSpecManifest := "cluster_ubuntu_ntp_config.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_ubuntu_ntp_config_cp.yaml")
	test.AssertContentToFile(t, string(md), "testdata/expected_results_ubuntu_ntp_config_md.yaml")
}

func TestProviderGenerateDeploymentFileForBottlerocketWithBottlerocketSettingsConfig(t *testing.T) {
	clusterSpecManifest := "cluster_bottlerocket_settings_config.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}

	cp, md, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	fmt.Println(string(md))

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_bottlerocket_settings_config_cp.yaml")
	test.AssertContentToFile(t, string(md), "testdata/expected_results_bottlerocket_settings_config_md.yaml")
}

func TestTinkerbellProviderGenerateCAPISpecForCreateWithPodIAMConfig(t *testing.T) {
	clusterSpecManifest := "cluster_tinkerbell_awsiam.yaml"
	mockCtrl := gomock.NewController(t)
	docker := stackmocks.NewMockDocker(mockCtrl)
	helm := stackmocks.NewMockHelm(mockCtrl)
	kubectl := mocks.NewMockProviderKubectlClient(mockCtrl)
	stackInstaller := stackmocks.NewMockStackInstaller(mockCtrl)
	writer := filewritermocks.NewMockFileWriter(mockCtrl)
	cluster := &types.Cluster{Name: "test"}
	forceCleanup := false

	clusterSpec := givenClusterSpec(t, clusterSpecManifest)
	clusterSpec.Cluster.Spec.PodIAMConfig = &v1alpha1.PodIAMConfig{ServiceAccountIssuer: "https://test"}

	datacenterConfig := givenDatacenterConfig(t, clusterSpecManifest)
	machineConfigs := givenMachineConfigs(t, clusterSpecManifest)
	ctx := context.Background()

	provider := newProvider(datacenterConfig, machineConfigs, clusterSpec.Cluster, writer, docker, helm, kubectl, forceCleanup)
	provider.stackInstaller = stackInstaller

	stackInstaller.EXPECT().CleanupLocalBoots(ctx, forceCleanup)

	if err := provider.SetupAndValidateCreateCluster(ctx, clusterSpec); err != nil {
		t.Fatalf("failed to setup and validate: %v", err)
	}
	cp, _, err := provider.GenerateCAPISpecForCreate(context.Background(), cluster, clusterSpec)
	if err != nil {
		t.Fatalf("failed to generate cluster api spec contents: %v", err)
	}

	test.AssertContentToFile(t, string(cp), "testdata/expected_results_tinkerbell_pod_iam_config.yaml")
}
