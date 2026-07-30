import { useCallback, useEffect, useState } from 'react';
import { Button, Form, Input, InputNumber, Modal, Select, Space, message } from 'antd';
import { PlusOutlined, EditOutlined } from '@ant-design/icons';
import type { Issue } from '../types';
import type { CreateIssueRequest, UpdateIssueRequest } from '../api';

const STATUS_OPTIONS = [
  { label: 'Open', value: 'open' },
  { label: 'In Progress', value: 'in_progress' },
  { label: 'Blocked', value: 'blocked' },
  { label: 'Closed', value: 'closed' },
  { label: 'Deferred', value: 'deferred' },
];

const TYPE_OPTIONS = [
  { label: 'Feature', value: 'feature' },
  { label: 'Bug', value: 'bug' },
  { label: 'Task', value: 'task' },
  { label: 'Epic', value: 'epic' },
];

type CreateMode = {
  mode: 'create';
  onSubmit: (req: CreateIssueRequest) => Promise<void>;
};

type EditMode = {
  mode: 'edit';
  issue: Issue;
  onSubmit: (req: UpdateIssueRequest) => Promise<void>;
};

type Props = (CreateMode | EditMode) & {
  readOnly?: boolean;
};

export function IssueForm({ readOnly, ...props }: Props) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (props.mode === 'edit') {
      form.setFieldsValue({
        title: props.issue.title,
        description: props.issue.description || '',
        status: props.issue.status,
        priority: props.issue.priority,
        issue_type: props.issue.issue_type || '',
        labels: props.issue.labels || [],
        assignee: props.issue.assignee || '',
        acceptance_criteria: props.issue.acceptance_criteria || '',
      });
    } else {
      form.resetFields();
    }
    setDirty(false);
  }, [props, form]);

  const handleSubmit = useCallback(async () => {
    try {
      const values = await form.validateFields();
      setLoading(true);

      if (props.mode === 'create') {
        const req: CreateIssueRequest = {
          title: values.title,
          description: values.description || undefined,
          priority: values.priority ?? 3,
          issue_type: values.issue_type || undefined,
          labels: values.labels?.length ? values.labels : undefined,
        };
        await props.onSubmit(req);
        message.success('Issue created');
      } else {
        const req: UpdateIssueRequest = {};
        if (values.title !== props.issue.title) req.title = values.title;
        if (values.description !== (props.issue.description || '')) req.description = values.description;
        if (values.status !== props.issue.status) req.status = values.status;
        if (values.priority !== props.issue.priority) req.priority = values.priority;
        if (values.issue_type !== (props.issue.issue_type || '')) req.issue_type = values.issue_type;
        if (values.assignee !== (props.issue.assignee || '')) req.assignee = values.assignee;
        if (values.acceptance_criteria !== (props.issue.acceptance_criteria || '')) {
          req.acceptance_criteria = values.acceptance_criteria;
        }
        // Labels comparison
        const oldLabels = (props.issue.labels || []).sort().join(',');
        const newLabels = (values.labels || []).sort().join(',');
        if (oldLabels !== newLabels) req.labels = values.labels;

        await props.onSubmit(req);
        message.success('Issue updated');
      }
      setDirty(false);
    } catch (e) {
      if (e instanceof Error) {
        message.error(e.message);
      }
    } finally {
      setLoading(false);
    }
  }, [form, props]);

  if (readOnly) return null;

  const isEdit = props.mode === 'edit';

  return (
    <Form
      form={form}
      layout="vertical"
      size="small"
      onValuesChange={() => setDirty(true)}
      style={{ padding: 16 }}
    >
      <Form.Item
        name="title"
        label="Title"
        rules={[{ required: true, message: 'Title is required' }]}
      >
        <Input placeholder="Issue title" />
      </Form.Item>

      <Form.Item name="description" label="Description">
        <Input.TextArea rows={4} placeholder="Description..." />
      </Form.Item>

      {isEdit && (
        <Form.Item name="acceptance_criteria" label="Acceptance Criteria">
          <Input.TextArea rows={3} placeholder="Acceptance criteria..." />
        </Form.Item>
      )}

      <Space style={{ width: '100%' }} direction="vertical" size={8}>
        <div style={{ display: 'flex', gap: 8 }}>
          <Form.Item name="status" label="Status" style={{ flex: 1, marginBottom: 0 }}>
            <Select options={STATUS_OPTIONS} />
          </Form.Item>
          <Form.Item name="priority" label="Priority" style={{ width: 100, marginBottom: 0 }}>
            <InputNumber min={0} max={4} />
          </Form.Item>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <Form.Item name="issue_type" label="Type" style={{ flex: 1, marginBottom: 0 }}>
            <Select options={TYPE_OPTIONS} allowClear placeholder="Type" />
          </Form.Item>
          <Form.Item name="assignee" label="Assignee" style={{ flex: 1, marginBottom: 0 }}>
            <Input placeholder="Assignee" />
          </Form.Item>
        </div>
        <Form.Item name="labels" label="Labels" style={{ marginBottom: 0 }}>
          <Select mode="tags" placeholder="Add labels" />
        </Form.Item>
      </Space>

      <div style={{ marginTop: 16, display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
        <Button
          type="primary"
          onClick={handleSubmit}
          loading={loading}
          icon={isEdit ? <EditOutlined /> : <PlusOutlined />}
        >
          {isEdit ? 'Save Changes' : 'Create Issue'}
        </Button>
      </div>
    </Form>
  );
}

// Standalone create button that opens a modal
type CreateButtonProps = {
  onCreate: (req: CreateIssueRequest) => Promise<void>;
  readOnly?: boolean;
};

export function CreateIssueButton({ onCreate, readOnly }: CreateButtonProps) {
  const [open, setOpen] = useState(false);

  if (readOnly) return null;

  return (
    <>
      <Button type="primary" icon={<PlusOutlined />} onClick={() => setOpen(true)} size="small">
        New Issue
      </Button>
      <Modal
        title="Create New Issue"
        open={open}
        onCancel={() => setOpen(false)}
        footer={null}
        width={600}
      >
        <IssueForm
          mode="create"
          onSubmit={async (req) => {
            await onCreate(req);
            setOpen(false);
          }}
        />
      </Modal>
    </>
  );
}
