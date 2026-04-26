'use strict';

const XML_TOOL_SEGMENT_TAGS = [
  '<tool_calls>', '<tool_calls\n', '<tool_calls ',
  '<|dsml|tool_calls>', '<|dsml|tool_calls\n', '<|dsml|tool_calls ',
  '<dsml|tool_calls>', '<dsml|tool_calls\n', '<dsml|tool_calls ',
  '<dsmltool_calls>', '<dsmltool_calls\n', '<dsmltool_calls ',
];

const XML_TOOL_OPENING_TAGS = [
  '<tool_calls',
  '<|dsml|tool_calls',
  '<dsml|tool_calls',
  '<dsmltool_calls',
];

const XML_TOOL_CLOSING_TAGS = [
  '</tool_calls>',
  '</|dsml|tool_calls>',
  '</dsml|tool_calls>',
  '</dsmltool_calls>',
];

module.exports = {
  XML_TOOL_SEGMENT_TAGS,
  XML_TOOL_OPENING_TAGS,
  XML_TOOL_CLOSING_TAGS,
};
