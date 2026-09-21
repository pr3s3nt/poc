# Orchestrator References

Thư mục này chứa tài liệu kỹ thuật chỉ đọc, không phải code sản phẩm hay nguồn yêu cầu chuẩn.

## Deployment delta problem statement

[`deployment-delta-problem-v4.md`](deployment-delta-problem-v4.md) là đề bài
Humanitec-style ban đầu đã được chuyển khỏi repository root trong đợt làm sạch
tài liệu. Nó chỉ giữ provenance/reference; specification của orchestrator vẫn
nằm trong `orchestrator_docs/usecase/`.

## Humanitec planner challenge v4

`humanitec-planner-challenge-v4/` là bản copy cố định dùng để nghiên cứu UC-06 trước khi realization. Phải đọc đề, nguồn, source, grader và fixtures; sau đó ghi ánh xạ sang domain orchestrator tại `orchestrator_docs/implementation/uc06-planner-reference.md`.

Không sửa bản reference để phù hợp thiết kế. Những khái niệm thuộc challenge harness phải được tách khỏi kiến trúc sản phẩm.

Reference `final_idp` từng dùng cho quy trình và cách tổ chức artifact UP không
còn được clone trong workspace. Provenance được cố định tại ADR-005:
`https://github.com/pr3s3nt/final_idp.git`, branch `uc03-impl`, commit
`e6dc6631ba9db1bc80e2ff56380f50db99d490f9`.
