"use strict";(self.webpackChunkminder_docs=self.webpackChunkminder_docs||[]).push([[9706],{36893:(e,n,t)=>{t.r(n),t.d(n,{assets:()=>a,contentTitle:()=>l,default:()=>u,frontMatter:()=>s,metadata:()=>o,toc:()=>d});var r=t(74848),i=t(28453);const s={title:"Alerting",sidebar_position:50},l="Alerts from Minder",o={id:"understand/alerts",title:"Alerting",description:"Minder issues alerts to notify you when the state of your software supply chain does not meet the criteria that you've defined in your profile.",source:"@site/docs/understand/alerts.md",sourceDirName:"understand",slug:"/understand/alerts",permalink:"/understand/alerts",draft:!1,unlisted:!1,tags:[],version:"current",sidebarPosition:50,frontMatter:{title:"Alerting",sidebar_position:50},sidebar:"minder",previous:{title:"Providers",permalink:"/understand/providers"},next:{title:"Repository registration",permalink:"/understand/repository_registration"}},a={},d=[{value:"Enabling alerts in a profile",id:"enabling-alerts-in-a-profile",level:3},{value:"Alert types",id:"alert-types",level:2},{value:"Configuring alerts in profiles",id:"configuring-alerts-in-profiles",level:2}];function c(e){const n={a:"a",code:"code",em:"em",h1:"h1",h2:"h2",h3:"h3",li:"li",p:"p",pre:"pre",ul:"ul",...(0,i.R)(),...e.components};return(0,r.jsxs)(r.Fragment,{children:[(0,r.jsx)(n.h1,{id:"alerts-from-minder",children:"Alerts from Minder"}),"\n",(0,r.jsxs)(n.p,{children:["Minder issues ",(0,r.jsx)(n.em,{children:"alerts"})," to notify you when the state of your software supply chain does not meet the criteria that you've defined in your ",(0,r.jsx)(n.a,{href:"/understand/profiles",children:"profile"}),"."]}),"\n",(0,r.jsx)(n.p,{children:"Alerts are a core feature of Minder providing you with notifications about the status of your registered\nrepositories. These alerts automatically open and close based on the evaluation of the rules defined in your profiles."}),"\n",(0,r.jsx)(n.p,{children:"When a rule fails, Minder opens an alert to bring your attention to the non-compliance issue. Conversely, when the\nrule evaluation passes, Minder will automatically close any previously opened alerts related to that rule."}),"\n",(0,r.jsx)(n.p,{children:"In the alert, you'll be able to see details such as:"}),"\n",(0,r.jsxs)(n.ul,{children:["\n",(0,r.jsx)(n.li,{children:"The repository that is affected"}),"\n",(0,r.jsx)(n.li,{children:"The rule type that failed"}),"\n",(0,r.jsx)(n.li,{children:"The profile that the rule belongs to"}),"\n",(0,r.jsx)(n.li,{children:"Guidance on how to remediate and also fix the issue"}),"\n",(0,r.jsx)(n.li,{children:"Severity of the issue. The severity of the alert is based on what is set in the rule type definition."}),"\n"]}),"\n",(0,r.jsx)(n.h3,{id:"enabling-alerts-in-a-profile",children:"Enabling alerts in a profile"}),"\n",(0,r.jsx)(n.p,{children:'To activate the alert feature within a profile, you need to adjust the YAML definition.\nSpecifically, you should set the alert parameter to "on":'}),"\n",(0,r.jsx)(n.pre,{children:(0,r.jsx)(n.code,{className:"language-yaml",children:'alert: "on"\n'})}),"\n",(0,r.jsx)(n.p,{children:"Enabling alerts at the profile level means that for any rules included in the profile, alerts will be generated for\nany rule failures. For better clarity, consider this rule snippet:"}),"\n",(0,r.jsx)(n.pre,{children:(0,r.jsx)(n.code,{className:"language-yaml",children:'---\nversion: v1\ntype: rule-type\nname: sample_rule\ndef:\n  alert:\n      type: security_advisory\n      security_advisory:\n        severity: "medium"\n'})}),"\n",(0,r.jsxs)(n.p,{children:["In this example, the ",(0,r.jsx)(n.code,{children:"sample_rule"})," defines an alert action that creates a medium severity security advisory in the\nrepository for any non-compliant repositories."]}),"\n",(0,r.jsx)(n.p,{children:"Now, let's see how this works in practice within a profile. Consider the following profile configuration with alerts\nturned on:"}),"\n",(0,r.jsx)(n.pre,{children:(0,r.jsx)(n.code,{className:"language-yaml",children:'version: v1\ntype: profile\nname: sample-profile\ncontext:\n  provider: github\nalert: "on"\nrepository:\n  - type: sample_rule\n    def:\n      enabled: true\n'})}),"\n",(0,r.jsxs)(n.p,{children:["In this profile, all repositories that do not meet the conditions specified in the ",(0,r.jsx)(n.code,{children:"sample_rule"})," will automatically\ngenerate security advisories."]}),"\n",(0,r.jsx)(n.h2,{id:"alert-types",children:"Alert types"}),"\n",(0,r.jsx)(n.p,{children:"Minder supports alerts of type GitHub Security Advisory."}),"\n",(0,r.jsx)(n.p,{children:"The following is an example of how the alert definition looks like for a give rule type:"}),"\n",(0,r.jsx)(n.pre,{children:(0,r.jsx)(n.code,{className:"language-yaml",children:'---\nversion: v1\ntype: rule-type\nname: artifact_signature\n...\ndef:\n  # Defines the configuration for alerting on the rule\n  alert:\n    type: security_advisory\n    security_advisory:\n      severity: "medium"\n'})}),"\n",(0,r.jsx)(n.h2,{id:"configuring-alerts-in-profiles",children:"Configuring alerts in profiles"}),"\n",(0,r.jsxs)(n.p,{children:["Alerts are configured in the ",(0,r.jsx)(n.code,{children:"alert"})," section of the profile yaml file. The following example shows how to configure\nalerts for a profile:"]}),"\n",(0,r.jsx)(n.pre,{children:(0,r.jsx)(n.code,{className:"language-yaml",children:'---\nversion: v1\ntype: profile\nname: github-profile\ncontext:\n  provider: github\nalert: "on"\nrepository:\n  - type: secret_scanning\n    def:\n      enabled: true\n'})}),"\n",(0,r.jsxs)(n.p,{children:["The ",(0,r.jsx)(n.code,{children:"alert"})," section can be configured with the following values: ",(0,r.jsx)(n.code,{children:"on"})," (default), ",(0,r.jsx)(n.code,{children:"off"})," and ",(0,r.jsx)(n.code,{children:"dry_run"}),". Dry run would be\nuseful for testing. In ",(0,r.jsx)(n.code,{children:"dry_run"})," Minder will process the alert conditions and output the resulted REST call, but it\nwon't execute it."]})]})}function u(e={}){const{wrapper:n}={...(0,i.R)(),...e.components};return n?(0,r.jsx)(n,{...e,children:(0,r.jsx)(c,{...e})}):c(e)}},28453:(e,n,t)=>{t.d(n,{R:()=>l,x:()=>o});var r=t(96540);const i={},s=r.createContext(i);function l(e){const n=r.useContext(s);return r.useMemo((function(){return"function"==typeof e?e(n):{...n,...e}}),[n,e])}function o(e){let n;return n=e.disableParentContext?"function"==typeof e.components?e.components(i):e.components||i:l(e.components),r.createElement(s.Provider,{value:n},e.children)}}}]);