import { Save } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

export function SettingsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Settings</h1>
        <p className="text-muted-foreground">
          Configure your Conductor instance
        </p>
      </div>

      <Tabs defaultValue="general" className="space-y-4">
        <TabsList>
          <TabsTrigger value="general">General</TabsTrigger>
          <TabsTrigger value="integrations">Integrations</TabsTrigger>
          <TabsTrigger value="notifications">Notifications</TabsTrigger>
          <TabsTrigger value="api">API</TabsTrigger>
        </TabsList>

        <TabsContent value="general" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>General Settings</CardTitle>
              <CardDescription>
                Basic configuration for your Conductor instance
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <label htmlFor="instance-name" className="text-sm font-medium">
                    Instance Name
                  </label>
                  <Input
                    id="instance-name"
                    placeholder="My Conductor Instance"
                    defaultValue="Production Conductor"
                  />
                </div>
                <div className="space-y-2">
                  <label htmlFor="timezone" className="text-sm font-medium">
                    Timezone
                  </label>
                  <Input
                    id="timezone"
                    placeholder="UTC"
                    defaultValue="America/New_York"
                  />
                </div>
              </div>

              <div className="space-y-2">
                <label htmlFor="default-timeout" className="text-sm font-medium">
                  Default Test Timeout (minutes)
                </label>
                <Input
                  id="default-timeout"
                  type="number"
                  placeholder="30"
                  defaultValue="30"
                  className="max-w-[200px]"
                />
                <p className="text-xs text-muted-foreground">
                  Maximum time a test run can execute before being cancelled
                </p>
              </div>

              <div className="space-y-2">
                <label htmlFor="retention" className="text-sm font-medium">
                  Data Retention (days)
                </label>
                <Input
                  id="retention"
                  type="number"
                  placeholder="90"
                  defaultValue="90"
                  className="max-w-[200px]"
                />
                <p className="text-xs text-muted-foreground">
                  How long to keep test run data and artifacts
                </p>
              </div>

              <Button>
                <Save className="mr-2 h-4 w-4" />
                Save Changes
              </Button>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Agent Configuration</CardTitle>
              <CardDescription>
                Default settings for test execution agents
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <label htmlFor="heartbeat-interval" className="text-sm font-medium">
                    Heartbeat Interval (seconds)
                  </label>
                  <Input
                    id="heartbeat-interval"
                    type="number"
                    placeholder="30"
                    defaultValue="30"
                  />
                </div>
                <div className="space-y-2">
                  <label htmlFor="heartbeat-timeout" className="text-sm font-medium">
                    Heartbeat Timeout (seconds)
                  </label>
                  <Input
                    id="heartbeat-timeout"
                    type="number"
                    placeholder="90"
                    defaultValue="90"
                  />
                </div>
              </div>

              <Button>
                <Save className="mr-2 h-4 w-4" />
                Save Changes
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="integrations" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>GitHub Integration</CardTitle>
              <CardDescription>
                Connect Conductor to your GitHub repositories
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <label htmlFor="github-app-id" className="text-sm font-medium">
                  GitHub App ID
                </label>
                <Input
                  id="github-app-id"
                  placeholder="123456"
                  type="password"
                />
              </div>
              <div className="space-y-2">
                <label htmlFor="github-private-key" className="text-sm font-medium">
                  Private Key
                </label>
                <Input
                  id="github-private-key"
                  placeholder="-----BEGIN RSA PRIVATE KEY-----"
                  type="password"
                />
              </div>
              <div className="space-y-2">
                <label htmlFor="github-webhook-secret" className="text-sm font-medium">
                  Webhook Secret
                </label>
                <Input
                  id="github-webhook-secret"
                  placeholder="webhook-secret"
                  type="password"
                />
              </div>
              <Button>
                <Save className="mr-2 h-4 w-4" />
                Save Changes
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="notifications" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Notification Settings</CardTitle>
              <CardDescription>
                Configure how you receive notifications about test runs
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <label htmlFor="slack-webhook" className="text-sm font-medium">
                  Slack Webhook URL
                </label>
                <Input
                  id="slack-webhook"
                  placeholder="https://hooks.slack.com/services/..."
                  type="password"
                />
              </div>
              <Button>
                <Save className="mr-2 h-4 w-4" />
                Save Changes
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="api" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>API Keys</CardTitle>
              <CardDescription>
                Manage API keys for programmatic access
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <p className="text-sm text-muted-foreground">
                No API keys configured. Create an API key to access the Conductor API programmatically.
              </p>
              <Button>Generate API Key</Button>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
