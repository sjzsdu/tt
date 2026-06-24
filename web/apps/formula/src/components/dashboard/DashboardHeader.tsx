import { ApartmentOutlined, CopyOutlined, PoweroffOutlined, UnorderedListOutlined } from '@ant-design/icons';
import { Button, Card, Flex, Segmented, Space, Tag, Tooltip, Typography } from 'antd';
import type { DashboardView, FormulaDashboardSnapshot } from '../../types';
import { statusIcon, statusLabel, statusTone } from '../../utils/status';

export function DashboardHeader({
  snapshot,
  view,
  theme,
  onViewChange,
  onThemeChange,
  onCopyRunID,
  onRequestStop,
  onOpenFinalReport,
}: {
  snapshot: FormulaDashboardSnapshot;
  view: DashboardView;
  theme: 'light' | 'dark';
  onViewChange: (view: DashboardView) => void;
  onThemeChange: (theme: 'light' | 'dark') => void;
  onCopyRunID: () => void;
  onRequestStop: () => void;
  onOpenFinalReport: () => void;
}) {
  return (
    <Card className="dashboard-header-card">
      <Flex justify="space-between" align="flex-start" gap="large" wrap="wrap">
        <Space direction="vertical" size={8} className="dashboard-header-copy">
          <Typography.Text type="secondary" className="dashboard-kicker">tt formula dashboard</Typography.Text>
          <Flex align="center" gap="small" wrap="wrap">
            <Typography.Title level={1} className="dashboard-title">{snapshot.recipe_name}</Typography.Title>
            <Tag color={statusTone[snapshot.status] || 'processing'} icon={statusIcon(snapshot.status)}>
              {statusLabel(snapshot.status)}
            </Tag>
          </Flex>
          <Typography.Paragraph type="secondary" className="dashboard-description">
            {snapshot.description || 'Live execution control room for formula runs.'}
          </Typography.Paragraph>
          {snapshot.run_id && (
            <Tooltip title="Copy run id">
              <Button icon={<CopyOutlined />} onClick={onCopyRunID} className="run-id-button">
                Run ID <Typography.Text code>{snapshot.run_id}</Typography.Text>
              </Button>
            </Tooltip>
          )}
        </Space>

        <Space direction="vertical" align="end" size="middle">
          <Segmented
            size="small"
            value={theme}
            onChange={value => onThemeChange(value as 'light' | 'dark')}
            options={[{ label: 'Light', value: 'light' }, { label: 'Dark', value: 'dark' }]}
          />
          <Segmented
            value={view}
            onChange={value => onViewChange(value as DashboardView)}
            options={[
              { label: 'List', value: 'list', icon: <UnorderedListOutlined /> },
              { label: 'Graph', value: 'graph', icon: <ApartmentOutlined /> },
            ]}
          />
          <Button
            danger
            icon={<PoweroffOutlined />}
            disabled={snapshot.status !== 'running' || snapshot.stop_requested}
            onClick={onRequestStop}
          >
            {snapshot.stop_requested ? 'Stop requested' : 'Stop after current iteration'}
          </Button>
          <Button type="primary" disabled={!snapshot.final_output} onClick={onOpenFinalReport}>
            {snapshot.final_output ? 'Open final report' : 'Waiting for final report'}
          </Button>
        </Space>
      </Flex>
    </Card>
  );
}
