package main

import (
	"context"
	"strconv"
	"testing"

	"github.com/graph-gophers/graphql-go"
	"github.com/mattermost/mattermost-plugin-playbooks/client"
	"github.com/mattermost/mattermost-plugin-playbooks/server/app"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraphQLPlaybooks(t *testing.T) {
	e := Setup(t)
	e.CreateBasic()

	t.Run("basic get", func(t *testing.T) {
		var pbResultTest struct {
			Data struct {
				Playbook struct {
					ID    string
					Title string
				}
			}
		}
		testPlaybookQuery := `
		query Playbook($id: String!) {
			playbook(id: $id) {
				id
				title
			}
		}
		`
		err := e.PlaybooksAdminClient.DoGraphql(context.Background(), &client.GraphQLInput{
			Query:         testPlaybookQuery,
			OperationName: "Playbook",
			Variables:     map[string]interface{}{"id": e.BasicPlaybook.ID},
		}, &pbResultTest)
		require.NoError(t, err)

		assert.Equal(t, e.BasicPlaybook.ID, pbResultTest.Data.Playbook.ID)
		assert.Equal(t, e.BasicPlaybook.Title, pbResultTest.Data.Playbook.Title)
	})

	t.Run("list", func(t *testing.T) {
		var pbResultTest struct {
			Data struct {
				Playbooks []struct {
					ID    string
					Title string
				}
			}
		}
		testPlaybookQuery := `
		query Playbooks {
			playbooks {
				id
				title
			}
		}
		`
		err := e.PlaybooksAdminClient.DoGraphql(context.Background(), &client.GraphQLInput{
			Query:         testPlaybookQuery,
			OperationName: "Playbooks",
		}, &pbResultTest)
		require.NoError(t, err)

		assert.Len(t, pbResultTest.Data.Playbooks, 3)
	})

	t.Run("playbook mutate", func(t *testing.T) {
		newUpdatedTitle := "graphqlmutatetitle"

		err := gqlTestPlaybookUpdate(e, t, e.BasicPlaybook.ID, map[string]interface{}{"title": newUpdatedTitle})
		require.NoError(t, err)

		updatedPlaybook, err := e.PlaybooksAdminClient.Playbooks.Get(context.Background(), e.BasicPlaybook.ID)
		require.NoError(t, err)

		require.Equal(t, newUpdatedTitle, updatedPlaybook.Title)
	})

	t.Run("update playbook no permissions to broadcast", func(t *testing.T) {
		err := gqlTestPlaybookUpdate(e, t, e.BasicPlaybook.ID, map[string]interface{}{"broadcastChannelIDs": []string{e.BasicPrivateChannel.Id}})
		require.Error(t, err)
	})

	t.Run("update playbook without modifying broadcast channel ids without permission. should succeed because no modification.", func(t *testing.T) {
		e.BasicPlaybook.BroadcastChannelIDs = []string{e.BasicPrivateChannel.Id}
		err := e.PlaybooksAdminClient.Playbooks.Update(context.Background(), *e.BasicPlaybook)
		require.NoError(t, err)

		err = gqlTestPlaybookUpdate(e, t, e.BasicPlaybook.ID, map[string]interface{}{"description": "unrelatedupdate"})
		require.NoError(t, err)
	})

	t.Run("update playbook with too many webhoooks", func(t *testing.T) {
		urls := []string{}
		for i := 0; i < 65; i++ {
			urls = append(urls, "http://localhost/"+strconv.Itoa(i))
		}
		err := gqlTestPlaybookUpdate(e, t, e.BasicPlaybook.ID, map[string]interface{}{
			"webhookOnCreationEnabled": true,
			"webhookOnCreationURLs":    urls,
		})
		require.Error(t, err)
	})

	t.Run("change default owner", func(t *testing.T) {
		err := gqlTestPlaybookUpdate(e, t, e.BasicPlaybook.ID, map[string]interface{}{
			"defaultOwnerID": e.RegularUser.Id,
		})
		require.NoError(t, err)

		err = gqlTestPlaybookUpdate(e, t, e.BasicPlaybook.ID, map[string]interface{}{
			"defaultOwnerID": e.RegularUserNotInTeam.Id,
		})
		require.Error(t, err)
	})
}

func gqlTestPlaybookUpdate(e *TestEnvironment, t *testing.T, playbookID string, updates map[string]interface{}) error {
	testPlaybookMutateQuery :=
		`
mutation UpdatePlaybook($id: String!, $updates: PlaybookUpdates!) {
  updatePlaybook(id: $id, updates: $updates)
}
		`
	var response graphql.Response
	err := e.PlaybooksClient.DoGraphql(context.Background(), &client.GraphQLInput{
		Query:         testPlaybookMutateQuery,
		OperationName: "UpdatePlaybook",
		Variables:     map[string]interface{}{"id": playbookID, "updates": updates},
	}, &response)

	if len(response.Errors) != 0 {
		return errors.Errorf("graphql failure %+v", response.Errors)
	}

	return err
}

func TestGraphQLPlaybooksMetrics(t *testing.T) {
	e := Setup(t)
	e.CreateBasic()

	t.Run("metrics get", func(t *testing.T) {
		var pbResultTest struct {
			Data struct {
				Playbook struct {
					ID      string
					Title   string
					Metrics []client.PlaybookMetricConfig
				}
			}
		}
		testPlaybookQuery :=
			`
	query Playbook($id: String!) {
		playbook(id: $id) {
			id
			metrics {
				id
				title
				description
				type
				target
			}
		}
	}
	`
		err := e.PlaybooksAdminClient.DoGraphql(context.Background(), &client.GraphQLInput{
			Query:         testPlaybookQuery,
			OperationName: "Playbook",
			Variables:     map[string]interface{}{"id": e.BasicPlaybook.ID},
		}, &pbResultTest)
		require.NoError(t, err)

		require.Len(t, pbResultTest.Data.Playbook.Metrics, len(e.BasicPlaybook.Metrics))
		require.Equal(t, e.BasicPlaybook.Metrics[0].Title, pbResultTest.Data.Playbook.Metrics[0].Title)
		require.Equal(t, e.BasicPlaybook.Metrics[0].Type, pbResultTest.Data.Playbook.Metrics[0].Type)
		require.Equal(t, e.BasicPlaybook.Metrics[0].Target, pbResultTest.Data.Playbook.Metrics[0].Target)
	})

	t.Run("add metric", func(t *testing.T) {
		testAddMetricQuery := `
		mutation AddMetric($playbookID: String!, $title: String!, $description: String!, $type: String!, $target: Int) {
			addMetric(playbookID: $playbookID, title: $title, description: $description, type: $type, target: $target)
		}
		`
		var response graphql.Response
		err := e.PlaybooksClient.DoGraphql(context.Background(), &client.GraphQLInput{
			Query:         testAddMetricQuery,
			OperationName: "AddMetric",
			Variables: map[string]interface{}{
				"playbookID":  e.BasicPlaybook.ID,
				"title":       "New Metric",
				"description": "the description",
				"type":        app.MetricTypeDuration,
			},
		}, &response)
		require.NoError(t, err)
		require.Empty(t, response.Errors)

		updatedPlaybook, err := e.PlaybooksAdminClient.Playbooks.Get(context.Background(), e.BasicPlaybook.ID)
		require.NoError(t, err)

		require.Len(t, updatedPlaybook.Metrics, 2)
		assert.Equal(t, updatedPlaybook.Metrics[1].Title, "New Metric")
	})

	t.Run("update metric", func(t *testing.T) {
		testUpdateMetricQuery := `
		mutation UpdateMetric($id: String!, $title: String, $description: String, $target: Int) {
			updateMetric(id: $id, title: $title, description: $description, target: $target)
		}
		`

		var response graphql.Response
		err := e.PlaybooksClient.DoGraphql(context.Background(), &client.GraphQLInput{
			Query:         testUpdateMetricQuery,
			OperationName: "UpdateMetric",
			Variables: map[string]interface{}{
				"id":    e.BasicPlaybook.Metrics[0].ID,
				"title": "Updated Title",
			},
		}, &response)
		require.NoError(t, err)
		require.Empty(t, response.Errors)

		updatedPlaybook, err := e.PlaybooksAdminClient.Playbooks.Get(context.Background(), e.BasicPlaybook.ID)
		require.NoError(t, err)

		require.Len(t, updatedPlaybook.Metrics, 2)
		assert.Equal(t, "Updated Title", updatedPlaybook.Metrics[0].Title)
	})

	t.Run("delete metric", func(t *testing.T) {
		testDeleteMetricQuery := `
		mutation DeleteMetric($id: String!) {
			deleteMetric(id: $id)
		}
		`
		var response graphql.Response
		err := e.PlaybooksClient.DoGraphql(context.Background(), &client.GraphQLInput{
			Query:         testDeleteMetricQuery,
			OperationName: "DeleteMetric",
			Variables: map[string]interface{}{
				"id": e.BasicPlaybook.Metrics[0].ID,
			},
		}, &response)
		require.NoError(t, err)
		require.Empty(t, response.Errors)

		updatedPlaybook, err := e.PlaybooksAdminClient.Playbooks.Get(context.Background(), e.BasicPlaybook.ID)
		require.NoError(t, err)

		require.Len(t, updatedPlaybook.Metrics, 1)
	})
}
