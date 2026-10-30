export interface ContractSample {
  node: string;
  html: string;
}

export const CONTRACT_SAMPLES: ContractSample[] = [
  {
    node: "mention",
    html: '<p><span data-type="mention" data-id="0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b">@Ada Lovelace</span></p>',
  },
  {
    node: "pageLink",
    html: '<p><a data-type="page-link" data-page-id="0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c" href="/spaces/ENG/pages/0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c/getting-started">Getting started</a></p>',
  },
  {
    node: "panel",
    html: '<div data-type="panel" data-color="warning"><p>Heads up</p></div>',
  },
  {
    node: "expand",
    html: '<details data-type="expand"><summary>More detail</summary><div><p>Hidden text</p></div></details>',
  },
  {
    node: "status",
    html: '<p><span data-type="status" data-color="green">DONE</span></p>',
  },
  {
    node: "toc",
    html: '<div data-type="toc"></div>',
  },
  {
    node: "comment",
    html: '<p>Some <span data-type="comment" data-id="0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5d">anchored words</span> here</p>',
  },
  {
    node: "image",
    html: '<img src="/api/v1/attachments/0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5e/content" alt="diagram.png" width="50%" data-attachment-id="0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5e">',
  },
  {
    node: "taskList",
    html: '<ul data-type="taskList"><li data-type="taskItem" data-checked="true"><p>Done</p></li><li data-type="taskItem" data-checked="false"><p>Todo</p></li></ul>',
  },
  {
    node: "codeBlock",
    html: '<pre><code class="language-go">fmt.Println("hi")</code></pre>',
  },
  {
    node: "heading",
    html: '<h2 id="release-notes">Release notes</h2>',
  },
  {
    node: "link",
    html: '<p><a target="_blank" rel="noopener noreferrer" href="https://example.com">Example</a></p>',
  },
  {
    node: "table",
    html: '<table><tbody><tr><th colspan="1" rowspan="1"><p>Name</p></th><th colspan="1" rowspan="1"><p>Role</p></th></tr><tr><td colspan="1" rowspan="1"><p>Ada</p></td><td colspan="1" rowspan="1"><p>Engineer</p></td></tr></tbody></table>',
  },
  {
    node: "marks",
    html: "<p><strong>b</strong><em>i</em><u>u</u><s>s</s><code>c</code><mark>h</mark></p>",
  },
];
