package model

import "testing"

func TestLatestAtlantisPlanExtractsNewestPlanSummary(t *testing.T) {
	comments := []PRComment{
		{Author: "atlantis", Body: "Plan: 0 to add, 1 to change, 0 to destroy."},
		{Author: "alice", Body: "looks good"},
		{Author: "atlantis", Body: "Terraform will perform the following actions:\n\nPlan: 2 to add, 3 to change, 1 to destroy."},
	}

	plan := LatestAtlantisPlan(comments)
	if plan == nil {
		t.Fatal("plan = nil")
	}
	if plan.Add != 2 || plan.Change != 3 || plan.Destroy != 1 {
		t.Fatalf("counts = +%d ~%d -%d, want +2 ~3 -1", plan.Add, plan.Change, plan.Destroy)
	}
	if plan.Summary != "Plan: 2 to add, 3 to change, 1 to destroy" {
		t.Fatalf("summary = %q", plan.Summary)
	}
}

func TestLatestAtlantisPlanDetectsNoChanges(t *testing.T) {
	plan := LatestAtlantisPlan([]PRComment{{Author: "atlantis", Body: "No changes. Infrastructure is up-to-date."}})
	if plan == nil || !plan.NoChanges || plan.Summary != "No changes" {
		t.Fatalf("plan = %#v, want no changes", plan)
	}
}

func TestAtlantisPlanLayersExtractsChangedAndNoChangeLayers(t *testing.T) {
	body := "Ran plan for dir: `network/prod` workspace: `default`\nPlan: 1 to add, 2 to change, 0 to destroy.\n\nProject: app/dev\nNo changes. Infrastructure is up-to-date."

	layers := AtlantisPlanLayers(body)

	if len(layers) != 2 {
		t.Fatalf("layers = %#v, want 2", layers)
	}
	if layers[0].Name != "network/prod / default" || layers[0].Add != 1 || layers[0].Change != 2 || layers[0].Destroy != 0 {
		t.Fatalf("changed layer = %#v", layers[0])
	}
	if layers[1].Name != "app/dev" || !layers[1].NoChanges {
		t.Fatalf("no-change layer = %#v", layers[1])
	}
}

func TestAtlantisPlanLayersIgnoresPlanSummaryAndHiddenNoise(t *testing.T) {
	body := "# Plan Summary\n(1 unchanged block hidden)\nRan plan for dir: `network/prod` workspace: `default`\nTerraform will perform the following actions:\n(20 unchanged attributes hidden)\nPlan: 0 to add, 2 to change, 0 to destroy."

	layers := AtlantisPlanLayers(body)

	if len(layers) != 1 {
		t.Fatalf("layers = %#v, want one real layer", layers)
	}
	if layers[0].Name != "network/prod / default" || layers[0].Change != 2 {
		t.Fatalf("layer = %#v", layers[0])
	}
}
