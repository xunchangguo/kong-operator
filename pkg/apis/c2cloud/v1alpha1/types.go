package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type KongList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Kong `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Kong struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              KongSpec   `json:"spec"`
	Status            KongStatus `json:"status,omitempty"`
}

type KongSpec struct {
	KongURL  string `json: kongURL,required`
	Username string `json: username,omitempty`
	Password string `json: password,omitempty`
	//InsecureSkipVerify bool              `json: insecureSkipVerify,omitempty`
	LabelSelector map[string]string `json:"labelSelector,omitempty"`
}
type KongStatus struct {
	// Fill me
	TargetPods []*TargetPods `json: targetPods,omitempty`
}

type TargetPods struct {
	Pod     string `json:"pod,required"`
	Address string `json:"address,required"`
}
