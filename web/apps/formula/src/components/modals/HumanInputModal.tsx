import { useEffect, useState } from 'react';
import { Alert, Checkbox, Form, Input, Modal, Radio, Select } from 'antd';
import type { FormulaDashboardStep } from '../../types';

const { TextArea } = Input;

export function HumanInputModal({ step, onSubmit }: { step: FormulaDashboardStep | undefined; onSubmit: (stepID: string, values: Record<string, unknown>) => Promise<void> }) {
  const [form] = Form.useForm();
  const [submitting, setSubmitting] = useState(false);
  const request = step?.human_input_request;
  const fields = request?.form?.fields || [];
  const title = request?.form?.title || `Input needed${step?.title ? `: ${step.title}` : ''}`;

  useEffect(() => {
    if (!step) return;
    const initial: Record<string, unknown> = {};
    for (const field of fields) {
      if (field.default) initial[field.name] = field.default;
      if (field.type === 'checkbox' && !initial[field.name]) initial[field.name] = [];
    }
    form.setFieldsValue(initial);
  }, [step?.id]);

  const submit = async () => {
    if (!step) return;
    const values = await form.validateFields();
    setSubmitting(true);
    try {
      await onSubmit(step.id, values);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      open={!!step}
      title={title}
      okText={request?.form?.submit_label || 'Submit and resume'}
      onOk={submit}
      confirmLoading={submitting}
      closable={false}
      maskClosable={false}
      width={640}
      cancelButtonProps={{ style: { display: 'none' } }}
    >
      {request?.reason && <Alert type="warning" showIcon message={request.reason} style={{ marginBottom: 16 }} />}
      {request?.form?.description && <Alert type="info" showIcon message={request.form.description} style={{ marginBottom: 16 }} />}
      <Form form={form} layout="vertical" preserve={false} requiredMark="optional">
        {fields.map(field => {
          const rules = field.required ? [{ required: true, message: `${field.label || field.name} is required` }] : undefined;
          const options = (field.options || []).map(value => ({ label: value, value }));
          let control = <Input placeholder={field.placeholder} />;
          if (field.type === 'textarea') control = <TextArea rows={4} placeholder={field.placeholder} />;
          if (field.type === 'radio') control = <Radio.Group options={options} />;
          if (field.type === 'checkbox') control = <Checkbox.Group options={options} />;
          if (field.type === 'select') control = <Select options={options} placeholder={field.placeholder} />;
          return (
            <Form.Item key={field.name} name={field.name} label={field.label || field.name} rules={rules} extra={field.help}>
              {control}
            </Form.Item>
          );
        })}
      </Form>
    </Modal>
  );
}
