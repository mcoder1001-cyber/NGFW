import { RuleExpiryService } from './rule-expiry.service.js';

/** F-rule-expiry API module: the expiry warnings (the commit check is in commit/validation.service.ts). */
export const ruleExpiryFeature = {
  controllers: [],
  providers: [RuleExpiryService],
} as const;

export { expiryInPastIssues, expiringRules, expiryStates } from './expiry.js';
export { RuleExpiryService } from './rule-expiry.service.js';
