import React from 'react';
import {
  Page,
  PageSection,
  PageSectionVariants,
  Title,
  TextContent,
} from '@patternfly/react-core';

interface SupportPageLayoutProps {
  title: string;
  description?: string;
  children: React.ReactNode;
}

const SupportPageLayout: React.FC<SupportPageLayoutProps> = ({ title, description, children }) => (
  <Page>
    <PageSection variant={PageSectionVariants.light}>
      <TextContent>
        <Title headingLevel="h1">{title}</Title>
        {description && <p>{description}</p>}
      </TextContent>
    </PageSection>
    <PageSection>{children}</PageSection>
  </Page>
);

export default SupportPageLayout;
