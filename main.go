package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/InVisionApp/conjungo"
	"github.com/elliotchance/orderedmap"
	"github.com/spf13/viper"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sJson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/utils/strings/slices"
	k8sYaml "sigs.k8s.io/yaml"
)

func createClient(kubeConfigPath string) (kubernetes.Interface, error) {
	var kubeConfig *rest.Config
	if kubeConfigPath != "" {
		config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
		if err != nil {
			return nil, fmt.Errorf("unable to load kubeConfig from %s: %v", kubeConfigPath, err)
		}
		kubeConfig = config
	} else {
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("unable to load in-cluster config: %v", err)
		}
		kubeConfig = config
	}

	client, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create a client: %v", err)
	}
	return client, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

var (
	roleBaseFile       = "role_base.yaml"   // generated off kubectl api-resources
	roleMergedFile     = "role_merged.yaml" // Merge rules in Restrictions File (if present) and Base File
	restrictedRoleFile = os.Getenv("RESTRICTIONS")
)

func restrictions() []string {
	// Return the slice of restricted resources only if the file is present

	// Read restrictions Yaml file
	fileName := strings.Split(strings.TrimSpace(restrictedRoleFile), ".")
	viper.SetConfigName(fileName[0])
	viper.SetConfigType(fileName[1])
	viper.AddConfigPath("./")
	viper.AutomaticEnv()
	err := viper.ReadInConfig()
	if err != nil {
		fmt.Printf("File %v not found\n", restrictedRoleFile)
		os.Exit(1)
	}
	rules := viper.Get("rules")

	// fmt.Println(rules)                          // [map[apiGroups:[] resources:[secrets] verbs:[list watch]] map[apiGroups:[] resources:[pods/exec] verbs:[get]] map[apiGroups:[] resources:[namespaces] verbs:[get list patch update watch]] map[apiGroups:[metrics.k8s.io] resources:[nodes pods] verbs:[get]]]
	// fmt.Println(reflect.TypeOf(rules).String()) // []interface {}

	// Find all resources which are restricted
	var restricedResources []string
	for _, v := range rules.([]interface{}) {
		// fmt.Println(v) // map[apiGroups:[] resources:[secrets] verbs:[list watch]]
		for i, j := range v.(map[interface{}]interface{}) {
			if i == "resources" {
				// fmt.Println(j) [nodes pods]
				// fmt.Println(reflect.TypeOf(j).String()) // []interface {}
				for _, x := range j.([]interface{}) {
					restricedResources = append(restricedResources, fmt.Sprintf("%v", x))
				}
			}
		}
	}

	return restricedResources
}

func main() {
	// Any Restrictions
	restricedResources := []string{}
	if restrictedRoleFile != "" {
		restricedResources = restrictions()
	}

	var roleNameArg string
	flag.StringVar(&roleNameArg, "name", "restricted-cluster-role", "Override the name of the ClusterRole resource that is generated")

	var enableVerboseLogging bool
	flag.BoolVar(&enableVerboseLogging, "v", false, "Enable verbose logging")

	// Flags for kube config path
	var kubeConfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeConfig = flag.String("kubeConfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeConfig file")
	} else {
		kubeConfig = flag.String("kubeConfig", "", "absolute path to the kubeConfig file")
	}

	// Flags for InCluster
	var inClusterMode bool
	flag.BoolVar(&inClusterMode, "inClusterMode", true, "run in cluster mode")

	flag.Parse()

	// create the kubernetes client
	var clientSet kubernetes.Interface
	var err error

	if inClusterMode {
		clientSet, err = createClient("")
	} else {
		clientSet, err = createClient(*kubeConfig)
	}

	if err != nil {
		log.Fatalln(err)
	}

	apiResourceListArray, err := clientSet.Discovery().ServerResources()
	if err != nil {
		log.Printf("Error during server resource discovery, %s", err.Error())
		os.Exit(1)
	}

	resourcesByGroupAndVerb := orderedmap.NewOrderedMap()
	for _, apiResourceList := range apiResourceListArray {
		if enableVerboseLogging {
			log.Printf("Group: %s", apiResourceList.GroupVersion)
		}
		// rbac rules only look at API group names, not name & version
		groupOnly := strings.Split(apiResourceList.GroupVersion, "/")[0]
		// core API doesn't have a group "name". We set to "core" and replace at the end with a blank string in the rbac policy rule
		if apiResourceList.GroupVersion == "v1" {
			groupOnly = "core"
		}

		resourcesByVerb := make(map[string][]string)
		for _, apiResource := range apiResourceList.APIResources {
			if enableVerboseLogging {
				log.Printf("Resource: %s - Verbs: %s",
					apiResource.Name,
					apiResource.Verbs.String())
			}

			// Only add the resource if not restricted
			if slices.Contains(restricedResources, apiResource.Name) == false {
				verbList := make([]string, 0)
				for _, verb := range apiResource.Verbs {
					verbList = append(verbList, verb)
				}
				sort.Strings(verbList)
				verbString := strings.Join(verbList[:], ",")
				if value, ok := resourcesByVerb[verbString]; ok {
					resourcesByVerb[verbString] = append(value, apiResource.Name)
				} else {
					resourcesByVerb[verbString] = []string{apiResource.Name}
				}
			}
		}

		for k := range resourcesByVerb {
			var sb strings.Builder
			sb.WriteString(groupOnly)
			sb.WriteString("!")
			sb.WriteString(k)
			if resourceVal, exists := resourcesByGroupAndVerb.Get(sb.String()); exists {
				resourceSetMap := make(map[string]bool)
				for _, r := range resourceVal.([]string) {
					resourceSetMap[r] = true
				}
				for _, r := range resourcesByVerb[k] {
					resourceSetMap[r] = true
				}
				resourceSet := mapSetToList(resourceSetMap)
				resourcesByGroupAndVerb.Set(sb.String(), resourceSet)
			} else {
				resourcesByGroupAndVerb.Set(sb.String(), resourcesByVerb[k])
			}
		}
	}

	computedPolicyRules := make([]rbacv1.PolicyRule, 0)
	for _, k := range resourcesByGroupAndVerb.Keys() {
		splitKey := strings.Split(k.(string), "!")
		if len(splitKey) != 2 {
			log.Fatalf("Unexpected output from API: %s", k)
		}
		splitVerbList := strings.Split(splitKey[1], ",")
		apiGroup := splitKey[0]
		if splitKey[0] == "core" {
			apiGroup = ""
		}

		value, _ := resourcesByGroupAndVerb.Get(k)

		newPolicyRule := &rbacv1.PolicyRule{
			APIGroups: []string{apiGroup},
			Verbs:     splitVerbList,
			Resources: value.([]string),
		}
		computedPolicyRules = append(computedPolicyRules, *newPolicyRule)
	}
	completeRbac := &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRole",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: roleNameArg,
		},
		Rules: computedPolicyRules,
	}

	serializer := k8sJson.NewYAMLSerializer(k8sJson.DefaultMetaFactory, nil, nil)
	var writer = bytes.NewBufferString("")
	e := serializer.Encode(completeRbac, writer)
	if e != nil {
		log.Printf("Error encountered during YAML encoding, %s", e.Error())
		os.Exit(1)
	}

	if restrictedRoleFile == "" {
		fmt.Println(writer.String())
	}
	f, _ := os.Create(roleBaseFile)
	f.WriteString(writer.String())
	// fmt.Println(reflect.TypeOf(writer).String())                  // *bytes.Buffer
	// fmt.Println(reflect.TypeOf(writer.String()).String())         // string
	// fmt.Println(reflect.TypeOf(writer.String()).String())         // string
	// fmt.Println(reflect.TypeOf([]byte(writer.String())).String()) // []uint8

	// Add the rules of restrictions.yaml into the generated yaml. TODO: Break this into a separate function
	if restrictedRoleFile != "" {
		var base ClusterRole
		err = k8sYaml.Unmarshal([]byte(writer.String()), &base)
		if err != nil {
			fmt.Printf("Unable to Unmarshal the generated role, look at %v and error %v\n", roleBaseFile, err)
			os.Exit(1)
		}
		// fmt.Println(reflect.TypeOf(base).String()) // Type main.ClusterRole

		fileName := strings.Split(strings.TrimSpace(restrictedRoleFile), ".")
		restrictedData, _ := ioutil.ReadFile(fileName[0] + "." + fileName[1])
		var restricted ClusterRole
		err = k8sYaml.Unmarshal(restrictedData, &restricted)
		if err != nil {
			fmt.Printf("err: %v\n", err)
			return
		}
		if err != nil {
			fmt.Printf("Unable to Unmarshal the file %v. Error %v\n", restrictedRoleFile, err)
			os.Exit(1)
		}
		// fmt.Println(reflect.TypeOf(restricted).String()) // Type main.ClusterRole

		// Merge the two roles
		conjungo.Merge(&base, restricted, nil)

		// Convert struct ClusterRole to YAML
		base.Metadata.Name = roleNameArg // Merged cluster role get name as restrictions
		y, err := k8sYaml.Marshal(base)
		if err != nil {
			fmt.Printf("err: %v\n", err)
			return
		}
		fmt.Println(string(y))
		f, _ = os.Create(roleMergedFile)
		f.WriteString(string(y))
	}
}

// https://zhwt.github.io/yaml-to-go/ (repalace `yaml` with `json`) https://groups.google.com/g/golang-nuts/c/EnWMnDv3iyo
type ClusterRole struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Rules []struct {
		APIGroups []string `json:"apiGroups"`
		Resources []string `json:"resources"`
		Verbs     []string `json:"verbs"`
	} `json:"rules"`
}

func mapSetToList(initialMap map[string]bool) []string {
	list := make([]string, len(initialMap))
	i := 0
	for k := range initialMap {
		list[i] = k
		i++
	}
	return list
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
