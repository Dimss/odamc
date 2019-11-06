package dcwebhook

import (
	"encoding/json"
	"fmt"
	dcv1 "github.com/openshift/api/apps/v1"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()
)

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

const (
	addAntiAffinitiy string = `[
    { "op":"add","path":"/spec/template/metadata/annotations","value": {"scheduler.alpha.kubernetes.io/affinity": "{\"podAntiAffinity\": {\"requiredDuringSchedulingIgnoredDuringExecution\": [{\"labelSelector\": {\"matchExpressions\": [{\"key\": \"zone\",\"operator\": \"In\",\"values\":[\"z1\",\"z2\"]}]},\"topologyKey\": \"kubernetes.io/hostname\"}]}}"  } }
  ]`
)

func MutateDcWebHookHandler(w http.ResponseWriter, r *http.Request) {
	logrus.Info("Handling route webhook")
	var body []byte
	// Read request body
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}
	// K8S sends POST request with the admission webhook data,
	// the body can't be empty, but if it is,
	// further processing will be stopped and empty
	// admission response will be sent to K8S API
	if len(body) == 0 {
		errMessage := "The body is empty, can't proceed the request"
		sendAdmissionValidationResponse(w, false, errMessage)
		logrus.Errorf(errMessage)
		return
	}
	// This object gonna hold actual route
	var dc = dcv1.DeploymentConfig{}
	ar := v1beta1.AdmissionReview{}
	// Try to decode body into Admission Review object
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		logrus.Errorf("Error during deserializing request body: %v", err)
		sendAdmissionValidationResponse(w, false, "error during deserializing request body")
		return
	}
	// Try to unmarshal Admission Review raw object to DeploymentConfig
	if err := json.Unmarshal(ar.Request.Object.Raw, &dc); err != nil {
		errMessage := "Error during unmarshaling request body"
		logrus.Error(errMessage)
		sendAdmissionValidationResponse(w, false, errMessage)
		return
	}
	sendAdmissionMutationRouterResponse(ar.Request.UID, w)

	//if route.Spec.TLS != nil {
	//	sendAdmissionValidationResponse(w, true, "Router is secure, proceed request")
	//} else {
	//	sendAdmissionMutationRouterResponse(ar.Request.UID, w)
	//}
}

func sendAdmissionMutationRouterResponse(uuid types.UID, w http.ResponseWriter) {
	//v := map[string]string{"adsasd": "ads"}
	//patch := []map[string]string{{
	//	"op":    "add",
	//	"path":  "/spec/template/metadata/annotations",
	//	"value": ,
	//}}

	patch := []patchOperation{
		{
			Op:   "add",
			Path: "/spec/template/metadata/annotations",
			//Value: []map[string]string{{"adsasd": "ads"}},
			Value: `'{"podAntiAffinity":{"requiredDuringSchedulingIgnoredDuringExecution": [{"labelSelector":{"matchExpressions": [{"key": "zone", "operator": "In", "values":["z1","z2"]}]}, "topologyKey": "kubernetes.io/hostname"}]}}'`,
		},
	}
	//bytes, err := json.Marshal(patch)
	logrus.Infof("%v", patch)
	bytes := []byte(addAntiAffinitiy)
	// Compose admission response
	admissionResponse := &v1beta1.AdmissionResponse{}
	admissionResponse.Allowed = true
	admissionResponse.Patch = bytes
	pt := v1beta1.PatchTypeJSONPatch
	admissionResponse.PatchType = &pt
	// Compose admission review
	admissionReview := v1beta1.AdmissionReview{}
	admissionReview.Response = admissionResponse
	admissionReview.Response.UID = uuid

	resp, err := json.Marshal(admissionReview)
	if err != nil {
		logrus.Errorf("Error during marshaling admissionResponse object: %v", err)
		http.Error(w, fmt.Sprintf("Error during marshaling admissionResponse object: %w", err), http.StatusInternalServerError)
	}
	logrus.Info("Sending response to API server")
	if _, err := w.Write(resp); err != nil {
		logrus.Errorf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}

func sendAdmissionValidationResponse(w http.ResponseWriter, isAllowed bool, message string) {
	var admissionResponse *v1beta1.AdmissionResponse
	admissionResponse = &v1beta1.AdmissionResponse{Allowed: isAllowed, Result: &metav1.Status{Message: message}}
	admissionReview := v1beta1.AdmissionReview{}
	admissionReview.Response = admissionResponse
	resp, err := json.Marshal(admissionReview)
	if err != nil {
		logrus.Errorf("Error during marshaling admissionResponse object: %v", err)
		http.Error(w, fmt.Sprintf("Error during marshaling admissionResponse object: %w", err), http.StatusInternalServerError)
	}
	logrus.Info("Sending response to API server")
	if _, err := w.Write(resp); err != nil {
		logrus.Errorf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}
