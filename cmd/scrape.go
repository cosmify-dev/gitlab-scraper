/*
Copyright © 2025 Alexander Padberg <undefinedhuman>
*/
package cmd

import (
	"fmt"
	"maps"
	"os"

	"github.com/mitchellh/mapstructure"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

var (
	configFile     string
	accessToken    string
	pushGatewayURL string
)

type ProjectCountConfig struct {
	IncludeSubGroups *bool `json:"include_subgroups,omitempty"`
}

type MemberCountConfig struct{}

type GroupConfig struct {
	ID           string              `json:"id"`
	ProjectCount *ProjectCountConfig `json:"project_count,omitempty"`
	MemberCount  *MemberCountConfig  `json:"member_count,omitempty"`
}

type Config struct {
	DefaultLabels map[string]string `json:"default_labels"`
	Groups        []GroupConfig     `json:"groups"`
}

var scrapeCmd = &cobra.Command{
	Use:   "scrape",
	Short: "Scrape statisticsfrom GitLab",
	Long:  `This command scrapes statisticsfrom from GitLab based on the provided configuration file.`,
	Run: func(cmd *cobra.Command, args []string) {
		viper.SetConfigFile(configFile)
		if err := viper.ReadInConfig(); err != nil {
			fmt.Printf("Failed to read config file: %v\n", err)
			os.Exit(1)
		}

		accessToken := getRequiredValue("access_token", "GITLAB_ACCESS_TOKEN",
			"Please provide an access token using the --token flag or GITLAB_ACCESS_TOKEN environment variable")
		pushGatewayURL := getRequiredValue("push_gateway_url", "PUSHGATEWAY_URL",
			"Please provide a Push Gateway URL using the --pushgateway flag or PUSHGATEWAY_URL environment variable")

		var config Config
		err := viper.Unmarshal(&config, func(dc *mapstructure.DecoderConfig) {
			dc.TagName = "json"
		})
		if err != nil {
			fmt.Printf("Failed to unmarshal config: %v\n", err)
			os.Exit(1)
		}

		scrape(&config, accessToken, pushGatewayURL)
	},
}

func init() {
	rootCmd.AddCommand(scrapeCmd)
	scrapeCmd.Flags().StringVarP(&configFile, "config", "c", "", "config file (required)")
	scrapeCmd.Flags().StringVarP(&accessToken, "token", "t", "", "GitLab access token (optional, can also be set via GITLAB_ACCESS_TOKEN environment variable)")
	scrapeCmd.Flags().StringVarP(&pushGatewayURL, "pushgateway", "p", "", "Prometheus Push Gateway URL (optional, can also be set via PUSHGATEWAY_URL environment variable)")
	scrapeCmd.MarkFlagRequired("config")
}

func getRequiredValue(key, envVar, errMsg string) string {
	viper.BindEnv(key, envVar)
	value := viper.GetString(key)
	if value == "" {
		fmt.Println(errMsg)
		os.Exit(1)
	}
	return value
}

func scrape(config *Config, accessToken string, pushGatewayURL string) {
	git, err := gitlab.NewClient(accessToken)
	if err != nil {
		fmt.Printf("Failed to create client: %v\n", err)
		os.Exit(1)
	}

	pusher := push.New(pushGatewayURL, "gitlab_scrape")

	for _, group := range config.Groups {
		if group.ProjectCount != nil {
			projectCount := getProjectCount(git, group)
			fmt.Printf("Project count in group %s: %d\n", group.ID, projectCount)

			labels := mergeLabels(config.DefaultLabels, prometheus.Labels{"group_id": group.ID})

			projectCountGauge := prometheus.NewGauge(prometheus.GaugeOpts{
				Name:        "gitlab_group_project_count",
				Help:        "Number of projects in the GitLab group",
				ConstLabels: labels,
			})
			projectCountGauge.Set(float64(projectCount))

			pusher.Collector(projectCountGauge)
		}

		if group.MemberCount != nil {
			groupMembersCount := getGroupMembersCount(git, group)
			fmt.Printf("Group members count in group %s: %d\n", group.ID, groupMembersCount)

			labels := mergeLabels(config.DefaultLabels, prometheus.Labels{"group_id": group.ID})

			groupMembersCountGauge := prometheus.NewGauge(prometheus.GaugeOpts{
				Name:        "gitlab_group_members_count",
				Help:        "Number of members in the GitLab group",
				ConstLabels: labels,
			})
			groupMembersCountGauge.Set(float64(groupMembersCount))

			pusher.Collector(groupMembersCountGauge)
		}
	}

	if err := pusher.Push(); err != nil {
		fmt.Printf("Failed to push metrics to Push Gateway: %v\n", err)
		os.Exit(1)
	}
}

func getProjectCount(git *gitlab.Client, group GroupConfig) int {
	includeSubGroups := false
	if group.ProjectCount.IncludeSubGroups != nil {
		includeSubGroups = *group.ProjectCount.IncludeSubGroups
	}

	_, resp, err := git.Groups.ListGroupProjects(group.ID, &gitlab.ListGroupProjectsOptions{
		IncludeSubGroups: gitlab.Ptr(includeSubGroups),
		ListOptions: gitlab.ListOptions{
			Page:    1,
			PerPage: 1,
		},
		Simple: gitlab.Ptr(true),
	})
	if err != nil {
		fmt.Printf("Failed to list projects for group %s: %v\n", group.ID, err)
		os.Exit(1)
	}

	return resp.TotalItems
}

func getGroupMembersCount(git *gitlab.Client, group GroupConfig) int {
	options := &gitlab.ListGroupMembersOptions{
		ListOptions: gitlab.ListOptions{
			Page:    1,
			PerPage: 1,
		},
	}

	_, resp, err := git.Groups.ListGroupMembers(group.ID, options)
	if err != nil {
		fmt.Printf("Failed to list members for group %s: %v\n", group.ID, err)
		os.Exit(1)
	}
	return resp.TotalItems
}

func mergeLabels(labelSets ...prometheus.Labels) prometheus.Labels {
	merged := prometheus.Labels{}
	for _, labels := range labelSets {
		maps.Copy(merged, labels)
	}
	return merged
}
